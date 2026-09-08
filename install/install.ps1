[CmdletBinding()]
param(
    [string]$Manifest = "https://releases.overgent.com/current/update-manifest.json",
    [string]$Archive,
    [string]$BackendManifest,
    [string]$BackendArchive,
    [string]$DownloadOnly,
    [switch]$SkipBackend,
    [switch]$AllowUnsignedDevelopmentBuild
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 3

$SignerCertificateSha256 = "__OVERGENT_WINDOWS_SIGNER_CERT_SHA256__"
$UpdatePublicKey = "__OVERGENT_UPDATE_PUBLIC_KEY__"
$MaximumArtifactBytes = 250MB
$Temporary = Join-Path ([IO.Path]::GetTempPath()) ("overgent-install-" + [Guid]::NewGuid().ToString("N"))

function Fail([string]$Message) { throw "overgent installer: $Message" }

function Get-Source([string]$Source, [string]$Destination, [string]$Label) {
    if ([string]::IsNullOrWhiteSpace($Source)) { Fail "$Label source is empty" }
    if ([Uri]::IsWellFormedUriString($Source, [UriKind]::Absolute)) {
        if ($AllowUnsignedDevelopmentBuild) { Fail "-AllowUnsignedDevelopmentBuild accepts local copied files only; it never weakens a network install" }
        $Uri = [Uri]$Source
        if ($Uri.Scheme -ne "https") { Fail "$Label must use HTTPS or be a local path" }
        try {
            $Response = Invoke-WebRequest -Uri $Uri -OutFile $Destination -UseBasicParsing -MaximumRedirection 5 -PassThru
            $FinalUri = $null
            if ($Response.BaseResponse.PSObject.Properties.Name -contains "ResponseUri") {
                $FinalUri = $Response.BaseResponse.ResponseUri
            } elseif ($Response.BaseResponse.PSObject.Properties.Name -contains "RequestMessage" -and $null -ne $Response.BaseResponse.RequestMessage) {
                $FinalUri = $Response.BaseResponse.RequestMessage.RequestUri
            }
            if ($null -eq $FinalUri -or $FinalUri.Scheme -ne "https") { Fail "$Label redirected away from HTTPS" }
        } catch {
            Fail "could not download the $Label. On a connected machine use -DownloadOnly, copy the verified files here, then pass their paths explicitly. $($_.Exception.Message)"
        }
    } else {
        $Resolved = Resolve-Path -LiteralPath $Source -ErrorAction SilentlyContinue
        if ($null -eq $Resolved -or -not (Test-Path -LiteralPath $Resolved -PathType Leaf)) { Fail "$Label was not found at $Source" }
        Copy-Item -LiteralPath $Resolved -Destination $Destination
    }
}

function Read-Manifest([string]$Path, [string]$Platform) {
    $Info = Get-Item -LiteralPath $Path
    if ($Info.Length -gt 1MB) { Fail "release manifest exceeds the 1 MiB limit" }
    try { $Document = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -DateKind String } catch { Fail "release manifest is not valid JSON" }
    $Names = @($Document.PSObject.Properties.Name)
    foreach ($Required in @("schemaVersion", "version", "publishedAt", "assets", "signature")) {
        if ($Names -cnotcontains $Required) { Fail "release manifest is missing $Required" }
    }
    if (@($Names | Where-Object { $_ -cnotin @("schemaVersion", "version", "publishedAt", "assets", "signature") }).Count -ne 0) {
        Fail "release manifest contains an unknown field"
    }
    if ($Document.schemaVersion -ne 1) { Fail "release manifest schema version is not supported" }
    if ([string]$Document.version -cnotmatch '^v[0-9][0-9A-Za-z.+-]*$') { Fail "release manifest version is invalid" }
    if ([string]$Document.publishedAt -cnotmatch '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$') { Fail "release manifest publish time is invalid" }
    try {
        $null = [DateTimeOffset]::Parse([string]$Document.publishedAt, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind)
    } catch { Fail "release manifest publish time is invalid" }
    if ([string]$Document.signature -cnotmatch '^[A-Za-z0-9+/]{86}==$') { Fail "release manifest signature encoding is invalid" }
    $Assets = @($Document.assets.PSObject.Properties)
    if ($Assets.Count -lt 1 -or $Assets.Count -gt 12) { Fail "release manifest asset count is invalid" }
    $CanonicalAssets = [ordered]@{}
    foreach ($Entry in @($Assets | Sort-Object Name)) {
        if ([string]$Entry.Name -cnotmatch '^(darwin|linux|windows)_(amd64|arm64)$') { Fail "release manifest platform is invalid" }
        $Candidate = $Entry.Value
        $CandidateNames = @($Candidate.PSObject.Properties.Name)
        if ($CandidateNames.Count -ne 3 -or $CandidateNames -cnotcontains "url" -or $CandidateNames -cnotcontains "sha256" -or $CandidateNames -cnotcontains "size") {
            Fail "release asset fields are invalid"
        }
        if ([string]$Candidate.url -cnotmatch '^https://[^\s]+$') { Fail "release asset URL is not HTTPS" }
        if ([string]$Candidate.sha256 -cnotmatch '^[a-f0-9]{64}$') { Fail "release asset checksum is invalid" }
        $CandidateSize = [Int64]$Candidate.size
        if ($CandidateSize -lt 1 -or $CandidateSize -gt $MaximumArtifactBytes) { Fail "release asset size is invalid" }
        $CanonicalAssets[[string]$Entry.Name] = [ordered]@{ url = [string]$Candidate.url; sha256 = [string]$Candidate.sha256; size = $CandidateSize }
    }
    if (-not $AllowUnsignedDevelopmentBuild) {
        if ($UpdatePublicKey -like "__OVERGENT_*__") { Fail "release update public key was absent when this installer was rendered" }
        $OpenSsl = Get-Command openssl.exe -ErrorAction SilentlyContinue
        if ($null -eq $OpenSsl) { $OpenSsl = Get-Command openssl -ErrorAction SilentlyContinue }
        if ($null -eq $OpenSsl) { Fail "OpenSSL is required to verify Overgent release metadata; install it, then retry" }
        try {
            $RawKey = [Convert]::FromBase64String($UpdatePublicKey)
            $Signature = [Convert]::FromBase64String([string]$Document.signature)
        } catch { Fail "release manifest signature encoding is invalid" }
        if ($RawKey.Length -ne 32 -or $Signature.Length -ne 64) { Fail "release manifest signature is invalid" }
        [byte[]]$Prefix = 0x30,0x2a,0x30,0x05,0x06,0x03,0x2b,0x65,0x70,0x03,0x21,0x00
        $PublicKeyPath = Join-Path $Temporary ("manifest-" + [Guid]::NewGuid().ToString("N") + ".der")
        $SignaturePath = "$PublicKeyPath.sig"
        $PayloadPath = "$PublicKeyPath.payload"
        [IO.File]::WriteAllBytes($PublicKeyPath, [byte[]]($Prefix + $RawKey))
        [IO.File]::WriteAllBytes($SignaturePath, $Signature)
        $Payload = [ordered]@{
            schemaVersion = [int]$Document.schemaVersion
            version = [string]$Document.version
            publishedAt = [string]$Document.publishedAt
            assets = $CanonicalAssets
        } | ConvertTo-Json -Compress -Depth 5
        [IO.File]::WriteAllText($PayloadPath, $Payload, [Text.UTF8Encoding]::new($false))
        & $OpenSsl.Source pkeyutl -verify -pubin -inkey $PublicKeyPath -keyform DER -rawin -in $PayloadPath -sigfile $SignaturePath *> $null
        if ($LASTEXITCODE -ne 0) { Fail "release manifest signature verification failed" }
    }
    $Property = $Assets | Where-Object { $_.Name -ceq $Platform }
    if ($null -eq $Property) { Fail "release publishes no asset for $Platform" }
    $Asset = $Property.Value
    $AssetNames = @($Asset.PSObject.Properties.Name)
    if ($AssetNames.Count -ne 3 -or $AssetNames -cnotcontains "url" -or $AssetNames -cnotcontains "sha256" -or $AssetNames -cnotcontains "size") {
        Fail "release asset fields are invalid"
    }
    if ([string]$Asset.url -cnotmatch '^https://[^\s]+$') { Fail "release asset URL is not HTTPS" }
    if ([string]$Asset.sha256 -cnotmatch '^[a-f0-9]{64}$') { Fail "release asset checksum is invalid" }
    $Size = [Int64]$Asset.size
    if ($Size -lt 1 -or $Size -gt $MaximumArtifactBytes) { Fail "release asset size is invalid" }
    return [PSCustomObject]@{ Version = [string]$Document.version; Url = [string]$Asset.url; Sha256 = [string]$Asset.sha256; Size = $Size }
}

function Test-Artifact([string]$Path, $Asset, [string]$Label) {
    $Info = Get-Item -LiteralPath $Path
    if ($Info.Length -ne $Asset.Size) { Fail "$Label size verification failed" }
    $Hash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($Hash -cne $Asset.Sha256) { Fail "$Label checksum verification failed" }
}

function Test-SignedBinary([string]$Path, [string]$Label) {
    if ($SignerCertificateSha256 -like "__OVERGENT_*__") {
        if ($AllowUnsignedDevelopmentBuild) { return }
        Fail "Windows signing credentials were absent when this installer was rendered. Public Windows installation is unavailable until a signed candidate is qualified"
    }
    $Signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($Signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or $null -eq $Signature.SignerCertificate) {
        Fail "$Label Authenticode signature is not valid ($($Signature.Status))"
    }
    $CertificateHash = $Signature.SignerCertificate.GetCertHashString([Security.Cryptography.HashAlgorithmName]::SHA256).ToLowerInvariant()
    if ($CertificateHash -cne $SignerCertificateSha256) { Fail "$Label signer identity does not match Overgent's release signer" }
}

function Expand-ExactZip([string]$Path, [string]$Destination, [string[]]$Expected) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Zip = [IO.Compression.ZipFile]::OpenRead($Path)
    try {
        $Names = @($Zip.Entries | ForEach-Object { $_.FullName })
        $ActualList = (($Names | Sort-Object) -join "`n")
        $ExpectedList = (($Expected | Sort-Object) -join "`n")
        if ($ActualList -cne $ExpectedList) { Fail "archive contains unexpected entries" }
    } finally { $Zip.Dispose() }
    [IO.Compression.ZipFile]::ExtractToDirectory($Path, $Destination)
}

try {
    if ($PSVersionTable.PSVersion -lt [Version]"7.5") {
        Fail "PowerShell 7.5 or newer is required for strict release-manifest parsing"
    }
    if (-not [Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -notin @("AMD64", "x86")) {
        Fail "Windows ARM is not published because the pinned local backend has no Windows ARM artifact. Use an amd64 Windows machine or a hosted Project"
    }
    if ($AllowUnsignedDevelopmentBuild) {
        foreach ($Source in @($Manifest, $Archive, $(if (-not $SkipBackend) { $BackendManifest }), $(if (-not $SkipBackend) { $BackendArchive }))) {
            if (-not [string]::IsNullOrWhiteSpace($Source) -and [Uri]::IsWellFormedUriString($Source, [UriKind]::Absolute)) {
                Fail "-AllowUnsignedDevelopmentBuild accepts local copied files only; it never weakens a network install"
            }
        }
    } elseif ($SignerCertificateSha256 -like "__OVERGENT_*__") {
        Fail "Windows signing credentials were absent when this installer was rendered. Public Windows installation is unavailable until a signed candidate is qualified"
    }

    New-Item -ItemType Directory -Path $Temporary | Out-Null
    $ManifestPath = Join-Path $Temporary "update-manifest.json"
    Get-Source $Manifest $ManifestPath "signed CLI manifest"
    $CliAsset = Read-Manifest $ManifestPath "windows_amd64"
    $ArchivePath = Join-Path $Temporary "overgent.zip"
    Get-Source $(if ($Archive) { $Archive } else { $CliAsset.Url }) $ArchivePath "CLI archive"
    Test-Artifact $ArchivePath $CliAsset "CLI archive"
    $CliExpanded = Join-Path $Temporary "cli"
    New-Item -ItemType Directory -Path $CliExpanded | Out-Null
    Expand-ExactZip $ArchivePath $CliExpanded @("LICENSE", "NOTICE", "README.md", "overgent.exe")
    $StagedExecutable = Join-Path $CliExpanded "overgent.exe"
    Test-SignedBinary $StagedExecutable "Overgent executable"

    $BackendAsset = $null
    $BackendArchivePath = $null
    if (-not $SkipBackend) {
        if ([string]::IsNullOrWhiteSpace($BackendManifest)) { $BackendManifest = [Uri]::new([Uri]$CliAsset.Url, "backend-manifest.json").AbsoluteUri }
        $BackendManifestPath = Join-Path $Temporary "backend-manifest.json"
        Get-Source $BackendManifest $BackendManifestPath "signed backend manifest"
        $BackendAsset = Read-Manifest $BackendManifestPath "windows_amd64"
        if ($BackendAsset.Version -cne $CliAsset.Version) { Fail "CLI and backend manifests publish different versions" }
        $BackendArchivePath = Join-Path $Temporary "overgent-backend.zip"
        Get-Source $(if ($BackendArchive) { $BackendArchive } else { $BackendAsset.Url }) $BackendArchivePath "backend archive"
        Test-Artifact $BackendArchivePath $BackendAsset "backend archive"
    }

    if ($DownloadOnly) {
        New-Item -ItemType Directory -Force -Path $DownloadOnly | Out-Null
        Copy-Item $ManifestPath (Join-Path $DownloadOnly "update-manifest.json")
        Copy-Item $ArchivePath (Join-Path $DownloadOnly ([IO.Path]::GetFileName($CliAsset.Url)))
        if (-not $SkipBackend) {
            Copy-Item $BackendManifestPath (Join-Path $DownloadOnly "backend-manifest.json")
            Copy-Item $BackendArchivePath (Join-Path $DownloadOnly ([IO.Path]::GetFileName($BackendAsset.Url)))
        }
        Write-Host "Verified release files copied to $DownloadOnly. Copy them to the offline machine and pass their local paths explicitly."
        exit 0
    }

    $InstallRoot = Join-Path $env:LOCALAPPDATA "Programs\Overgent"
    New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
    $Installed = Join-Path $InstallRoot "overgent.exe"
    $Previous = "$Installed.previous"

    $BackendBinary = $null
    $BackendBundle = $null
    if (-not $SkipBackend) {
        $RuntimeRoot = Join-Path $env:APPDATA "Overgent\runtime\$($BackendAsset.Version)"
        if (-not (Test-Path -LiteralPath $RuntimeRoot -PathType Container)) {
            $RuntimeStage = Join-Path $Temporary "backend"
            New-Item -ItemType Directory -Path $RuntimeStage | Out-Null
            Expand-ExactZip $BackendArchivePath $RuntimeStage @("LICENSE.md", "backend-push.json", "convex-local-backend.exe")
            Test-SignedBinary (Join-Path $RuntimeStage "convex-local-backend.exe") "Local backend executable"
            New-Item -ItemType Directory -Force -Path (Split-Path $RuntimeRoot) | Out-Null
            Move-Item $RuntimeStage $RuntimeRoot
        } else {
            $RuntimeStage = Join-Path $Temporary "backend-check"
            New-Item -ItemType Directory -Path $RuntimeStage | Out-Null
            Expand-ExactZip $BackendArchivePath $RuntimeStage @("LICENSE.md", "backend-push.json", "convex-local-backend.exe")
            foreach ($Name in @("LICENSE.md", "backend-push.json", "convex-local-backend.exe")) {
                $InstalledRuntimeFile = Join-Path $RuntimeRoot $Name
                if (-not (Test-Path -LiteralPath $InstalledRuntimeFile -PathType Leaf)) {
                    Fail "installed backend runtime is incomplete; move $RuntimeRoot aside and reinstall"
                }
                $ExpectedHash = (Get-FileHash -LiteralPath (Join-Path $RuntimeStage $Name) -Algorithm SHA256).Hash
                $InstalledHash = (Get-FileHash -LiteralPath $InstalledRuntimeFile -Algorithm SHA256).Hash
                if ($ExpectedHash -cne $InstalledHash) { Fail "installed backend runtime differs from the signed $($BackendAsset.Version) release; move $RuntimeRoot aside and reinstall" }
            }
        }
        $BackendBinary = Join-Path $RuntimeRoot "convex-local-backend.exe"
        $BackendBundle = Join-Path $RuntimeRoot "backend-push.json"
        Test-SignedBinary $BackendBinary "Local backend executable"
    }

    $HadPrevious = Test-Path -LiteralPath $Installed -PathType Leaf
    if ($HadPrevious) {
        Remove-Item -LiteralPath $Previous -Force -ErrorAction SilentlyContinue
        Move-Item -LiteralPath $Installed -Destination $Previous
    }
    Copy-Item -LiteralPath $StagedExecutable -Destination $Installed
    try {
        if (-not $SkipBackend) {
            & $Installed backend install --binary $BackendBinary --bundle $BackendBundle | Out-Null
            if ($LASTEXITCODE -ne 0) { Fail "backend registration failed with exit code $LASTEXITCODE" }
        }
        & $Installed service install
        if ($LASTEXITCODE -ne 0) { Fail "service installation failed with exit code $LASTEXITCODE" }
    } catch {
        Remove-Item -LiteralPath $Installed -Force -ErrorAction SilentlyContinue
        if ($HadPrevious) {
            Move-Item -LiteralPath $Previous -Destination $Installed
            & $Installed service install *> $null
            if ($LASTEXITCODE -ne 0) { Write-Warning "The previous executable was restored, but its scheduled task needs manual repair." }
        }
        throw
    }

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $UserPathEntries = if ([string]::IsNullOrWhiteSpace($UserPath)) { @() } else { @($UserPath -split ';') }
    if ($UserPathEntries -notcontains $InstallRoot) {
        $NewUserPath = if ([string]::IsNullOrWhiteSpace($UserPath)) { $InstallRoot } else { $UserPath.TrimEnd(';') + ';' + $InstallRoot }
        [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    }
    Write-Host "Overgent $($CliAsset.Version) installed and its per-user scheduled task started."
    if (-not $SkipBackend) { Write-Host "The $($BackendAsset.Version) local backend was fetched separately and configured for local Projects." }
    Write-Host "Open a new terminal before using overgent from PATH."
} finally {
    Remove-Item -LiteralPath $Temporary -Recurse -Force -ErrorAction SilentlyContinue
}
