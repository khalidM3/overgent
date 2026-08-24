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

if (process.platform !== "darwin") {
  throw new Error("desktop preview build is currently supported only on macOS");
}

await rm(app, { recursive: true, force: true });
await mkdir(path.dirname(executable), { recursive: true });

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
  <key>CFBundleShortVersionString</key><string>${development ? "0.1.0-dev" : "0.1.0-preview"}</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>Copyright 2026 Stickguy contributors</string>
</dict></plist>
`;
await writeFile(path.join(app, "Contents", "Info.plist"), plist, { mode: 0o644 });

const sign = spawnSync("codesign", ["--force", "--sign", "-", app], { stdio: "inherit" });
if (sign.status !== 0) process.exit(sign.status ?? 1);

console.log(app);
