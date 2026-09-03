import { mkdirSync, realpathSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const values = new Map();
for (let index = 2; index < process.argv.length; index += 2) {
  const key = process.argv[index];
  const value = process.argv[index + 1];
  if (!key?.startsWith("--") || !value) throw new Error("usage: pnpm dev:agents -- --codex-root <worktree> --claude-root <worktree>");
  values.set(key, value);
}
if (!values.has("--codex-root") || !values.has("--claude-root")) {
  throw new Error("usage: pnpm dev:agents -- --codex-root <worktree> --claude-root <worktree>");
}
const codexRoot = realpathSync(values.get("--codex-root"));
const claudeRoot = realpathSync(values.get("--claude-root"));
if (codexRoot === claudeRoot) throw new Error("Codex and Claude require distinct Git worktree roots for separate attribution");

function gitCommonDirectory(directory) {
  const result = spawnSync("git", ["rev-parse", "--path-format=absolute", "--git-common-dir"], { cwd: directory, encoding: "utf8" });
  if (result.status !== 0) throw new Error(`${directory} is not a Git worktree`);
  return realpathSync(result.stdout.trim());
}
if (gitCommonDirectory(codexRoot) !== gitCommonDirectory(claudeRoot)) {
  throw new Error("Codex and Claude roots must be linked worktrees of the same Git repository");
}

const binary = path.join(root, "bin", "overgent");
mkdirSync(path.dirname(binary), { recursive: true });
const build = spawnSync("go", ["build", "-o", binary, "./cmd/overgent"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);
function run(arguments_, options = {}) {
  const result = spawnSync(binary, arguments_, { cwd: root, encoding: "utf8", ...options });
  if (result.status !== 0) throw new Error((result.stderr || result.stdout || `overgent ${arguments_.join(" ")} failed`).trim());
  return result.stdout.trim();
}
function workspaces() { return JSON.parse(run(["workspace", "list"])); }
function ensureWorkspace(directory) {
  let workspace = workspaces().find((candidate) => realpathSync(candidate.root) === directory);
  if (!workspace) {
    workspace = JSON.parse(run(["workspace", "add", "--development", "--root", directory]));
  }
  return workspace;
}

const codexWorkspace = ensureWorkspace(codexRoot);
const claudeWorkspace = ensureWorkspace(claudeRoot);
const codexStatus = JSON.parse(run(["setup", "codex", "--development", "--project-root", codexRoot]));
const claudeStatus = JSON.parse(run(["setup", "claude", "--development", "--project-root", claudeRoot]));
process.stdout.write(`${JSON.stringify({
  codex: { root: codexRoot, workspaceId: codexWorkspace.id, workstreamId: codexWorkspace.workstreamId, adapter: codexStatus, fidelity: "mcp_with_git_fallback" },
  claude: { root: claudeRoot, workspaceId: claudeWorkspace.id, workstreamId: claudeWorkspace.workstreamId, adapter: claudeStatus, fidelity: "mcp" },
}, null, 2)}\n`);
process.stdout.write("Restart already-running Codex or Claude sessions so they discover the new project MCP configuration.\n");
