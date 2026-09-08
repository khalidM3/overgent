import { useEffect, useState } from "react";
import { ArrowRight, Check, Copy, Download, GitMerge, Radar, ShieldCheck, Terminal, Waypoints } from "lucide-react";
import { BrandMark } from "./brand";
import "./landing.css";

type ReleaseState =
  | { status: "loading" | "unavailable" }
  | { status: "ready"; version: string; cliURLs: Record<string, string> };

interface ReleaseManifest {
  version?: unknown;
  assets?: unknown;
}

// Every download goes through overgent.com rather than a GitHub URL: the site
// redirects each of these to the matching asset on the latest release, so the
// link in a blog post or a README keeps working across releases.
const DESKTOP_URL = "https://overgent.com/download/macos";
const INSTALL_COMMAND = "curl -fsSL https://overgent.com/install.sh | sh";
const WINDOWS_INSTALL_COMMAND = "irm https://overgent.com/install.ps1 | iex";

interface Platform {
  id: string;
  name: string;
  detail: string;
  blurb: string;
  desktopURL: string;
  // cliKey indexes the update manifest, whose asset names goreleaser derives
  // from GOOS and GOARCH.
  cliKey: string;
  // qualified is false for a platform that builds and passes its tests but has
  // not been run on real hardware. It is the difference between "we shipped it"
  // and "we shipped it and know it works", and saying so is cheaper than a bug
  // report from somebody who assumed the former.
  qualified: boolean;
}

const PLATFORMS: Platform[] = [
  {
    id: "macos",
    name: "macOS",
    detail: "Apple silicon · macOS 12+",
    blurb: "Desktop app and CLI together, signed and notarized for direct distribution.",
    desktopURL: "https://overgent.com/download/macos",
    cliKey: "darwin_arm64",
    qualified: true,
  },
  {
    id: "linux",
    name: "Linux",
    detail: "x86-64 · GTK 3 and WebKitGTK",
    blurb: "Credentials in the Secret Service, the service on systemd --user, and the same bundled backend.",
    desktopURL: "https://overgent.com/download/linux",
    cliKey: "linux_amd64",
    qualified: false,
  },
  {
    id: "windows",
    name: "Windows",
    detail: "x86-64 · Windows 10 1809+",
    blurb: "Credentials in the Credential Manager, the service on Task Scheduler, IPC over a named pipe.",
    desktopURL: "https://overgent.com/download/windows",
    cliKey: "windows_amd64",
    qualified: false,
  },
];

export function LandingPage() {
  const [release, setRelease] = useState<ReleaseState>({ status: "loading" });

  useEffect(() => {
    let active = true;
    void fetch("/current/update-manifest.json", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error("release unavailable");
        const body = await response.json() as ReleaseManifest;
        const assets = body.assets && typeof body.assets === "object" ? body.assets as Record<string, unknown> : {};
        if (typeof body.version !== "string" || !/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(body.version)) {
          throw new Error("invalid release version");
        }
        // A platform whose asset is missing or not HTTPS simply gets no CLI
        // link. One absent architecture must not blank the whole page.
        const cliURLs: Record<string, string> = {};
        for (const platform of PLATFORMS) {
          const asset = assets[platform.cliKey];
          if (!asset || typeof asset !== "object") continue;
          const url = (asset as Record<string, unknown>).url;
          if (typeof url === "string" && url.startsWith("https://")) cliURLs[platform.id] = url;
        }
        if (active) setRelease({ status: "ready", version: body.version, cliURLs });
      })
      .catch(() => { if (active) setRelease({ status: "unavailable" }); });
    return () => { active = false; };
  }, []);

  const ready = release.status === "ready";
  return <div className="landing">
    <header className="landing-nav">
      <Wordmark />
      <nav aria-label="Main navigation">
        <a href="#how-it-works">How it works</a>
        <a href="#privacy">Privacy</a>
        <a href="#download">Download</a>
        <a className="landing-nav-action" href="/dashboard">Open dashboard <ArrowRight size={14} /></a>
      </nav>
    </header>

    <main>
      <section className="landing-hero">
        <div>
          <p className="landing-kicker"><span>Open source</span> Coordination for parallel agents</p>
          <h1>Keep every coding agent working from the same reality.</h1>
          <p className="landing-lede">Run local by default, invite teammates only when you choose, and bring your own model for semantic coordination.</p>
          <div className="landing-actions">
            <a className="landing-primary" href={ready ? DESKTOP_URL : "#download"}><Download size={16} /> {ready ? "Download for macOS" : "Get Overgent"}</a>
            <a className="landing-secondary" href="/dashboard">Open dashboard <ArrowRight size={15} /></a>
          </div>
          <p className="landing-trust"><ShieldCheck size={14} /> Raw source, prompts, diffs, credentials, and command output never cross the wire.</p>
        </div>

        <div className="landing-radar" aria-label="Example of Overgent coordinating three agent sessions">
          <div className="landing-radar-head"><span>Project / launch</span><span>Live coordination</span></div>
          <div className="landing-radar-row"><span className="landing-agent">CO</span><div><strong>Codex</strong><p>Refactoring the session boundary</p></div><code>auth/session.ts</code></div>
          <div className="landing-radar-finding">
            <strong>Claude is changing the same contract</strong>
            <p>Route the updated session shape before either agent builds on a stale assumption.</p>
          </div>
          <div className="landing-radar-row"><span className="landing-agent">CL</span><div><strong>Claude Code</strong><p>Updating the browser handshake</p></div><code>auth/session.ts</code></div>
          <div className="landing-radar-row quiet"><span className="landing-agent">CO</span><div><strong>Codex</strong><p>Writing the migration tests</p></div><code>tests/session_test.go</code></div>
        </div>
      </section>

      <section className="landing-section" id="how-it-works" aria-labelledby="how-heading">
        <h2 id="how-heading">Air traffic control around the tools you already use.</h2>
        <p className="landing-section-lede">Overgent coordinates the work. Codex and Claude Code still own their model loops, files, commands, and permissions.</p>
        <ol className="landing-steps">
          <li>
            <span className="landing-step-icon"><Waypoints size={17} /></span>
            <h3>Connect a project</h3>
            <p>Point one local service at a repository. Run a single session, or invite the rest of your team.</p>
          </li>
          <li>
            <span className="landing-step-icon"><Radar size={17} /></span>
            <h3>Work normally</h3>
            <p>Overgent builds a shared model from safe Git evidence and what each supported agent reports.</p>
          </li>
          <li>
            <span className="landing-step-icon"><GitMerge size={17} /></span>
            <h3>Correct course early</h3>
            <p>Collisions, stale contracts, and ready dependencies reach the affected work — not everybody.</p>
          </li>
        </ol>
      </section>

      <section className="landing-section landing-privacy" id="privacy" aria-labelledby="privacy-heading">
        <h2 id="privacy-heading">Share coordination facts, not the work itself.</h2>
        <p className="landing-section-lede">The local service can see what is changing, but the wire accepts only derived, structured coordination facts. Raw source, raw diffs, Git objects, prompts, transcripts, tool output, environment values, and credentials stay on your machine. Overgent is open source under Apache-2.0.</p>
      </section>

      <section className="landing-section" id="download" aria-labelledby="download-heading">
        <div className="landing-download-head">
          <h2 id="download-heading">Download</h2>
          <ReleaseStatus release={release} />
        </div>

        <ul className="landing-platforms">
          {PLATFORMS.map((platform) => <li className="landing-platform" key={platform.id}>
            <div>
              <h3>{platform.name} <span>{platform.detail}</span></h3>
              <p>{platform.blurb}</p>
              {!platform.qualified && <p className="landing-platform-note">
                Built and tested in CI, but not yet run on a {platform.name} machine by us. It should work; if it does not, please open an issue.
              </p>}
            </div>
            <div className="landing-platform-actions">
              {ready
                ? <>
                    <a className="landing-primary compact" href={platform.desktopURL}><Download size={15} /> Desktop app</a>
                    {release.cliURLs[platform.id] && <a className="landing-secondary compact" href={release.cliURLs[platform.id]}>CLI archive</a>}
                  </>
                : <span className="landing-platform-state">Awaiting the first signed build</span>}
            </div>
          </li>)}
        </ul>

        <div className="landing-install">
          <p>Prefer the terminal? The installer verifies the signed update manifest, archive hash, executable signature, and Apple Team ID before it changes anything.</p>
          <div className="landing-install-commands">
            <CopyCommand enabled={ready} />
            <p className="landing-install-alt">On Windows, in PowerShell:</p>
            <CopyCommand enabled={ready} command={WINDOWS_INSTALL_COMMAND} />
          </div>
        </div>
      </section>
    </main>

    <footer className="landing-footer">
      <Wordmark />
      <p>Coordination infrastructure for teams building with agents.</p>
      <a href="/dashboard">Dashboard</a>
    </footer>
  </div>;
}

function Wordmark() {
  return <a className="landing-brand" href="/" aria-label="Overgent home"><span aria-hidden="true"><BrandMark /></span><strong>overgent</strong></a>;
}

function ReleaseStatus({ release }: { release: ReleaseState }) {
  if (release.status === "ready") return <p className="landing-release ready"><Check size={14} /> <strong>{release.version}</strong> available now</p>;
  if (release.status === "loading") return <p className="landing-release">Checking the release channel</p>;
  return <p className="landing-release">First signed build in preparation</p>;
}

function CopyCommand({ enabled, command = INSTALL_COMMAND }: { enabled: boolean; command?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    if (!enabled) return;
    await navigator.clipboard.writeText(command);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1_500);
  };
  return <div className="landing-command-wrap">
    <div className="landing-command">
      <Terminal size={15} />
      <code>{command}</code>
      <button type="button" onClick={() => void copy()} disabled={!enabled} aria-label={copied ? "Install command copied" : "Copy install command"}>
        {copied ? <Check size={15} /> : <Copy size={15} />}
      </button>
    </div>
    {!enabled && <small>Live once the first signed release is promoted.</small>}
  </div>;
}
