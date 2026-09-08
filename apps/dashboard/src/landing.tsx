import { useEffect, useState } from "react";
import { Check, Copy, Download, ShieldCheck, Terminal } from "lucide-react";
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
const REPOSITORY_URL = "https://github.com/khalidM3/overgent";

interface Platform {
  id: string;
  name: string;
  detail: string;
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
  { id: "macos", name: "macOS", detail: "Apple silicon · macOS 12+", desktopURL: "https://overgent.com/download/macos", cliKey: "darwin_arm64", qualified: true },
  { id: "linux", name: "Linux", detail: "x86-64 · GTK 3", desktopURL: "https://overgent.com/download/linux", cliKey: "linux_amd64", qualified: false },
  { id: "windows", name: "Windows", detail: "x86-64 · Windows 10 1809+", desktopURL: "https://overgent.com/download/windows", cliKey: "windows_amd64", qualified: false },
];

export function LandingPage() {
  const [release, setRelease] = useState<ReleaseState>({ status: "loading" });
  const stillOnly = usePrefersReducedMotion();

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
        <a href={REPOSITORY_URL}>GitHub</a>
        <a href="#download">Download</a>
      </nav>
    </header>

    <main>
      <section className="landing-hero">
        <h1>Keep every coding agent working from the same reality.</h1>
        <p className="landing-lede">Overgent catches conflicting assumptions and overlapping work while your agents are still working, and tells only the session that needs to know.</p>
        <div className="landing-actions">
          <a className="landing-primary" href={ready ? DESKTOP_URL : "#download"}><Download size={16} /> {ready ? "Download for macOS" : "Get Overgent"}</a>
          <a className="landing-secondary" href="#download">Linux and Windows</a>
        </div>
        <p className="landing-trust"><ShieldCheck size={14} /> Local by default. Raw source, prompts, diffs, credentials, and command output never cross the wire.</p>
      </section>

      {/* The product showing itself. These are captures of the real workroom, so
          the page no longer needs a hand-drawn imitation of it beside the
          headline, nor a row of icons explaining what the picture already says. */}
      <figure className="landing-shot">
        {stillOnly
          ? <img src="/media/workroom-collision.png" alt="Two agent sessions reported as changing the same file, with each session's goal and the evidence behind the finding." />
          : <video
              src="/media/workroom.webm"
              poster="/media/workroom-collision.png"
              autoPlay
              loop
              muted
              playsInline
              preload="metadata"
              aria-label="The Overgent workroom: a collision between two live agent sessions is opened, then a contract change that a session had already read."
            />}
        <figcaption>Two live sessions on one shared path, then a contract that moved under a session which had already read it — old signature, new signature, and who changed it.</figcaption>
      </figure>

      <section className="landing-section" id="download" aria-labelledby="download-heading">
        <div className="landing-download-head">
          <h2 id="download-heading">Download</h2>
          <ReleaseStatus release={release} />
        </div>

        <ul className="landing-platforms">
          {PLATFORMS.map((platform) => <li className="landing-platform" key={platform.id}>
            <div>
              <h3>{platform.name} <span>{platform.detail}</span></h3>
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
      <p>Open source under Apache-2.0.</p>
      <a href={REPOSITORY_URL}>GitHub</a>
    </footer>
  </div>;
}

/**
 * The hero loop is decorative motion, so a visitor who has asked their system
 * for less of it gets the still frame instead of an autoplaying video.
 */
function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(query.matches);
    const onChange = (event: MediaQueryListEvent) => setReduced(event.matches);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);
  return reduced;
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
