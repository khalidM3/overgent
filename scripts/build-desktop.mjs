import { copyFile, mkdir, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const desktop = path.join(root, "apps", "desktop");
const output = path.join(desktop, "build", "bin");
const development = process.argv.includes("--development");

// The three platforms Wails v3 gives a native webview. Anything else builds the
// main_unsupported.go stub, which is not something worth packaging.
const platform = process.platform;
if (!["darwin", "linux", "win32"].includes(platform)) {
  throw new Error(`desktop builds are supported on macOS, Linux and Windows; this is ${platform}`);
}

// One identity, spelled once. These have to agree with desktop_production.go
// and desktop_development.go, which is what keeps a build's bundle identifier,
// URL scheme, .desktop basename and single-instance lock from drifting apart.
const productName = development ? "Overgent Dev" : "Overgent";
const applicationID = development ? "com.overgent.app.development" : "com.overgent.app";
const entryName = development ? "overgent-dev" : "overgent";
const urlScheme = development ? "overgent-dev" : "overgent";
const executableName = development ? "overgent-desktop-dev" : "overgent-desktop";
const windows = platform === "win32";
const exeSuffix = windows ? ".exe" : "";

// Where the built application lives, and where it keeps what it ships.
// bundledResourceDirectory() in apps/desktop answers the same question at
// runtime; the two are one contract and both name it in one place.
const layout = {
  darwin: () => {
    const app = path.join(output, `${productName}.app`);
    return { app, executable: path.join(app, "Contents", "MacOS", executableName), resources: path.join(app, "Contents", "Resources") };
  },
  linux: () => {
    const app = path.join(output, entryName);
    return { app, executable: path.join(app, executableName), resources: path.join(app, "resources") };
  },
  win32: () => {
    const app = path.join(output, productName);
    return { app, executable: path.join(app, `${executableName}.exe`), resources: path.join(app, "resources") };
  },
}[platform]();

const { app, executable, resources } = layout;
const cli = path.join(resources, `overgent${exeSuffix}`);
const backendBinaryName = `convex-local-backend${exeSuffix}`;

await rm(app, { recursive: true, force: true });
await mkdir(path.dirname(executable), { recursive: true });
await mkdir(resources, { recursive: true });

if (!development) {
  const cliLdflags = [
    "-s", "-w",
    `-X main.version=${process.env.OVERGENT_VERSION ?? "dev"}`,
    `-X main.commit=${process.env.OVERGENT_COMMIT ?? "unknown"}`,
    `-X main.buildTime=${process.env.OVERGENT_BUILD_TIME ?? new Date().toISOString()}`,
    `-X main.updatePublicKey=${process.env.OVERGENT_UPDATE_PUBLIC_KEY ?? ""}`,
  ].join(" ");
  const cliBuild = spawnSync("go", ["build", "-trimpath", "-ldflags", cliLdflags, "-o", cli, "./cmd/overgent"], { cwd: root, stdio: "inherit" });
  if (cliBuild.status !== 0) process.exit(cliBuild.status ?? 1);
}

// A production build normally talks to the released hosted origin. A closed
// test deployment overrides it here rather than by editing the source, and the
// value has to be a clean HTTPS origin because that is all activation accepts.
const productionAPIOrigin = String(process.env.OVERGENT_PRODUCTION_API_ORIGIN ?? "").replace(/\/$/, "");
if (productionAPIOrigin) {
  let parsed;
  try { parsed = new URL(productionAPIOrigin); } catch { throw new Error("OVERGENT_PRODUCTION_API_ORIGIN must be a valid HTTPS URL"); }
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.search || parsed.hash || parsed.pathname !== "/") {
    throw new Error("OVERGENT_PRODUCTION_API_ORIGIN must be a clean HTTPS origin");
  }
}
const buildArguments = ["build"];
if (!development) buildArguments.push("-tags", "production", "-trimpath", "-ldflags", `-w -s${productionAPIOrigin ? ` -X main.apiBaseURL=${productionAPIOrigin}` : ""}`);
buildArguments.push("-o", executable, ".");

// Wails needs CGO on macOS and Linux; on Windows its backend is pure Go and
// loads WebView2 through go-winloader, which is why the cross-vet gate in CI
// can type-check the whole package for Windows from any runner.
const buildEnvironment = { ...process.env };
if (platform === "darwin") {
  Object.assign(buildEnvironment, {
    CGO_CFLAGS: `${process.env.CGO_CFLAGS ?? ""} -mmacosx-version-min=12.0`.trim(),
    CGO_LDFLAGS: `${process.env.CGO_LDFLAGS ?? ""} -mmacosx-version-min=12.0`.trim(),
    MACOSX_DEPLOYMENT_TARGET: "12.0",
    CGO_ENABLED: "1",
  });
}
if (platform === "linux") buildEnvironment.CGO_ENABLED = "1";
const build = spawnSync("go", buildArguments, { cwd: desktop, env: buildEnvironment, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);

// A production build carries the Convex backend it runs a local Project on, so
// a fresh install coordinates with no account, no network, and no Node. The
// binary and the release-time deploy payload are fetched and generated by
// scripts/fetch-backend.mjs and the release workflow; a development build skips
// them, and the app then offers only team Projects.
const backendDestination = path.join(resources, "backend");
if (!development) {
  const backendSource = path.join(desktop, "build", "backend");
  const payloadSource = path.join(desktop, "build", "backend-push.json");
  const binarySource = path.join(backendSource, backendBinaryName);
  const haveBinary = await stat(binarySource).then((info) => info.isFile()).catch(() => false);
  const havePayload = await stat(payloadSource).then((info) => info.isFile()).catch(() => false);
  if (!haveBinary || !havePayload) {
    // Shipping half the pair produces an app whose local-Project button fails
    // when pressed, which is worse than a build that says what is missing.
    throw new Error(`bundled backend is missing (expected ${backendBinaryName} and backend-push.json): run \`node scripts/fetch-backend.mjs\` and generate apps/desktop/build/backend-push.json (see scripts/backend-push.sh build)`);
  }
  await mkdir(backendDestination, { recursive: true });
  for (const entry of await readdir(backendSource)) {
    await copyFile(path.join(backendSource, entry), path.join(backendDestination, entry));
  }
  await copyFile(payloadSource, path.join(backendDestination, "backend-push.json"));
}

// Icons are rendered from the one canonical geometry rather than checked in per
// platform, so the mark cannot differ between a Dock icon, a .desktop entry and
// a link's shell icon. macOS keeps the committed .icns because iconutil's
// container is already generated and signed against.
async function renderIconSet() {
  const iconset = path.join(desktop, "build", "iconset");
  await rm(iconset, { recursive: true, force: true });
  const render = spawnSync("go", ["run", "./scripts/generate-brand-icons.go", iconset], { cwd: root, stdio: "inherit" });
  if (render.status !== 0) throw new Error("could not render the brand icon set");
  return iconset;
}

// A .desktop file is also written into the staged tree, beside the application,
// so a package can install it system-wide without re-deriving any of this. It
// is not the only registration: the app writes a per-user copy on every launch,
// because Exec= has to name where the executable actually ended up, and an
// AppImage or a tarball has no install step to write one at all.
function desktopEntry(execPath) {
  return [
    "[Desktop Entry]",
    "Type=Application",
    "Version=1.0",
    `Name=${productName}`,
    "Comment=Persistent coordination for teams working with coding agents",
    `Exec="${execPath.replace(/\\/g, "\\\\\\\\").replace(/"/g, '\\\\"').replace(/`/g, "\\\\`").replace(/\$/g, "\\\\$")}" %u`,
    `TryExec=${execPath}`,
    `Icon=${entryName}`,
    "Terminal=false",
    "Categories=Development;Utility;",
    `MimeType=x-scheme-handler/${urlScheme};`,
    `StartupWMClass=${entryName}`,
    "StartupNotify=true",
    "",
  ].join("\n");
}

// An .ico is a tiny container of images. Windows accepts PNG payloads directly
// for entries from Vista onwards, so the sizes the brand generator already
// produces are packed as-is rather than re-encoded as BMP.
function assembleICO(images) {
  const header = Buffer.alloc(6);
  header.writeUInt16LE(0, 0);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(images.length, 4);
  const directory = Buffer.alloc(16 * images.length);
  let offset = header.length + directory.length;
  images.forEach((image, index) => {
    const entry = index * 16;
    // 0 means 256 in this field, which is why it is one byte for a format that
    // routinely carries a 256-pixel icon.
    directory.writeUInt8(image.size >= 256 ? 0 : image.size, entry);
    directory.writeUInt8(image.size >= 256 ? 0 : image.size, entry + 1);
    directory.writeUInt8(0, entry + 2);
    directory.writeUInt8(0, entry + 3);
    directory.writeUInt16LE(1, entry + 4);
    directory.writeUInt16LE(32, entry + 6);
    directory.writeUInt32LE(image.data.length, entry + 8);
    directory.writeUInt32LE(offset, entry + 12);
    offset += image.data.length;
  });
  return Buffer.concat([header, directory, ...images.map((image) => image.data)]);
}

if (platform === "darwin") {
  const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleDisplayName</key><string>${productName}</string>
  <key>CFBundleExecutable</key><string>${executableName}</string>
  <key>CFBundleIdentifier</key><string>${applicationID}</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleName</key><string>${productName}</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>Overgent.icns</string>
  <key>CFBundleShortVersionString</key><string>${development ? "0.1.0-dev" : (process.env.OVERGENT_VERSION ?? "0.1.0-beta").replace(/^v/, "")}</string>
  <key>CFBundleVersion</key><string>${development ? "1" : process.env.OVERGENT_BUILD_NUMBER ?? "1"}</string>
  <!-- Enrollment needs the local service, which only the desktop app can reach.
       The hosted dashboard hands control back through this scheme so adding a
       Project is a click rather than a terminal command. Development registers
       its own scheme so a dev build never hijacks the released app's links.
       Linux and Windows have no manifest; the application writes their
       registration itself at launch (apps/desktop/deeplink_register_*.go). -->
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>${applicationID}</string>
      <key>CFBundleURLSchemes</key><array><string>${urlScheme}</string></array>
    </dict>
  </array>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>Copyright 2026 Overgent contributors</string>
</dict></plist>
`;
  await writeFile(path.join(app, "Contents", "Info.plist"), plist, { mode: 0o644 });
  await copyFile(path.join(root, "assets", "brand", "overgent-app-icon.icns"), path.join(resources, "Overgent.icns"));

  const identity = process.env.OVERGENT_CODESIGN_IDENTITY ?? "-";
  const signArguments = ["--force", "--sign", identity];
  if (identity !== "-") signArguments.push("--options", "runtime", "--timestamp");
  if (!development) {
    const cliSign = spawnSync("codesign", [...signArguments, cli], { stdio: "inherit" });
    if (cliSign.status !== 0) process.exit(cliSign.status ?? 1);
    // The nested backend is signed before the enclosing app, with the JIT
    // entitlement V8 needs: a hardened-runtime signature
    // without it fails at startup with "Failed to reserve virtual memory for
    // CodeRange". No other entitlement was needed.
    const entitlements = path.join(desktop, "build", "backend-entitlements.plist");
    await writeFile(entitlements, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>com.apple.security.cs.allow-jit</key><true/>
</dict></plist>
`, { mode: 0o644 });
    const backendSign = spawnSync("codesign", [...signArguments, "--entitlements", entitlements, path.join(backendDestination, backendBinaryName)], { stdio: "inherit" });
    if (backendSign.status !== 0) process.exit(backendSign.status ?? 1);
  }
  const sign = spawnSync("codesign", [...signArguments, app], { stdio: "inherit" });
  if (sign.status !== 0) process.exit(sign.status ?? 1);
} else {
  const iconset = await renderIconSet();
  const icons = path.join(resources, "icons");
  await mkdir(icons, { recursive: true });

  if (platform === "linux") {
    // The icon theme's largest standard directory; installDesktopIcon copies it
    // into hicolor/512x512 on first run, and a package installs it there too.
    await copyFile(path.join(iconset, "icon_512x512.png"), path.join(icons, `${entryName}.png`));
    // The staged entry names the installed location a package will put the
    // application at. OVERGENT_LINUX_PREFIX lets a deb or an AppImage recipe
    // say where that is; the default is the self-contained /opt layout, which
    // is what keeps `resources` beside the executable.
    const prefix = process.env.OVERGENT_LINUX_PREFIX ?? path.join("/opt", entryName);
    await writeFile(path.join(app, `${entryName}.desktop`), desktopEntry(path.join(prefix, executableName)), { mode: 0o644 });
  } else {
    const sizes = [16, 32, 64, 128, 256];
    const files = { 16: "icon_16x16.png", 32: "icon_32x32.png", 64: "icon_32x32@2x.png", 128: "icon_128x128.png", 256: "icon_256x256.png" };
    const images = [];
    for (const size of sizes) images.push({ size, data: await readFile(path.join(iconset, files[size])) });
    await writeFile(path.join(icons, `${entryName}.ico`), assembleICO(images), { mode: 0o644 });
    // Signing is the release workflow's job on every platform. On Windows that
    // is signtool over the executable and the staged CLI and backend, using a
    // certificate this script has no business holding.
  }
  await rm(iconset, { recursive: true, force: true });
}

console.log(app);
