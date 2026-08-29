import { mkdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const desktop = path.join(root, "apps", "desktop");
const output = path.join(desktop, "build", "bin");
const development = process.argv.includes("--development");
const productName = development ? "Stickguy Dev" : "Stickguy";
const app = path.join(output, `${productName}.app`);
const executableName = development ? "stickguy-desktop-dev" : "stickguy-desktop";
const executable = path.join(app, "Contents", "MacOS", executableName);
const resources = path.join(app, "Contents", "Resources");
const cli = path.join(resources, "stickguy");

if (process.platform !== "darwin") {
  throw new Error("desktop preview build is currently supported only on macOS");
}

await rm(app, { recursive: true, force: true });
await mkdir(path.dirname(executable), { recursive: true });
await mkdir(resources, { recursive: true });

if (!development) {
  const cliLdflags = [
    "-s", "-w",
    `-X main.version=${process.env.STICKGUY_VERSION ?? "dev"}`,
    `-X main.commit=${process.env.STICKGUY_COMMIT ?? "unknown"}`,
    `-X main.buildTime=${process.env.STICKGUY_BUILD_TIME ?? new Date().toISOString()}`,
    `-X main.updatePublicKey=${process.env.STICKGUY_UPDATE_PUBLIC_KEY ?? ""}`,
  ].join(" ");
  const cliBuild = spawnSync("go", ["build", "-trimpath", "-ldflags", cliLdflags, "-o", cli, "./cmd/stickguy"], { cwd: root, stdio: "inherit" });
  if (cliBuild.status !== 0) process.exit(cliBuild.status ?? 1);
}

const buildArguments = ["build"];
if (!development) buildArguments.push("-tags", "production", "-trimpath", "-ldflags", "-w -s");
buildArguments.push("-o", executable, ".");
const build = spawnSync("go", buildArguments, {
  cwd: desktop,
  env: {
    ...process.env,
    CGO_CFLAGS: `${process.env.CGO_CFLAGS ?? ""} -mmacosx-version-min=12.0`.trim(),
    CGO_LDFLAGS: `${process.env.CGO_LDFLAGS ?? ""} -mmacosx-version-min=12.0`.trim(),
    MACOSX_DEPLOYMENT_TARGET: "12.0",
  },
  stdio: "inherit",
});
if (build.status !== 0) process.exit(build.status ?? 1);

const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleDisplayName</key><string>${productName}</string>
  <key>CFBundleExecutable</key><string>${executableName}</string>
  <key>CFBundleIdentifier</key><string>${development ? "dev.stickguy.app.development" : "dev.stickguy.app"}</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleName</key><string>${productName}</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${development ? "0.1.0-dev" : (process.env.STICKGUY_VERSION ?? "0.1.0-beta").replace(/^v/, "")}</string>
  <key>CFBundleVersion</key><string>${development ? "1" : process.env.STICKGUY_BUILD_NUMBER ?? "1"}</string>
  <!-- Enrollment needs the local service, which only the desktop app can reach.
       The hosted dashboard hands control back through this scheme so adding a
       Project is a click rather than a terminal command. Development registers
       its own scheme so a dev build never hijacks the released app's links. -->
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>${development ? "dev.stickguy.app.development" : "dev.stickguy.app"}</string>
      <key>CFBundleURLSchemes</key><array><string>${development ? "stickguy-dev" : "stickguy"}</string></array>
    </dict>
  </array>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>Copyright 2026 Stickguy contributors</string>
</dict></plist>
`;
await writeFile(path.join(app, "Contents", "Info.plist"), plist, { mode: 0o644 });

const identity = process.env.STICKGUY_CODESIGN_IDENTITY ?? "-";
const signArguments = ["--force", "--sign", identity];
if (identity !== "-") signArguments.push("--options", "runtime", "--timestamp");
if (!development) {
  const cliSign = spawnSync("codesign", [...signArguments, cli], { stdio: "inherit" });
  if (cliSign.status !== 0) process.exit(cliSign.status ?? 1);
}
const sign = spawnSync("codesign", [...signArguments, app], { stdio: "inherit" });
if (sign.status !== 0) process.exit(sign.status ?? 1);

console.log(app);
