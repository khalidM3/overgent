import os from "node:os";
import path from "node:path";

/**
 * The profile the development stack runs against.
 *
 * It is deliberately not the default profile a released build uses. The two
 * were the same path until now, which meant `pnpm dev` inherited whatever a
 * release build had enrolled - including a Project bound to a server the
 * development backend is not - and an installed LaunchAgent running a release
 * binary was left "already managing" the profile, so the service code being
 * developed never ran at all.
 *
 * `OVERGENT_CONFIG_ROOT` still wins, so a throwaway profile is one variable
 * away; it must be absolute, the same rule `desktopConfigRoot` applies.
 */
export function devConfigRoot() {
  const override = (process.env.OVERGENT_CONFIG_ROOT ?? "").trim();
  if (override && path.isAbsolute(override)) return path.normalize(override);
  return path.join(os.homedir(), "Library", "Application Support", "Overgent Dev");
}
