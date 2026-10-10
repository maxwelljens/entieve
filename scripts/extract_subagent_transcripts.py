#!/usr/bin/env python3
"""Extract Entieve subagent conversations (including reasoning/thoughts) from
the local OpenCode service into docs/subagent-transcripts/ as Markdown.

This is a content-preserving archival tool: it fetches each session's message
transcript via `opencode api get /api/session/{id}/message` and writes it to
files WITHOUT printing any conversation content to stdout — only metadata
(session ids, block counts, file sizes) is reported.

Usage:  python3 scripts/extract_subagent_transcripts.py [--out docs/subagent-transcripts]
"""

import argparse
import datetime
import json
import re
import subprocess
import sys
from pathlib import Path

# Subagent sessions spawned by the Entieve coordinator session (this project).
# Explicit allowlist: identification is metadata-only, never content-based.
SESSION_IDS = [
    "ses_edef145f8ffeJ8MhWaMHPIyjQO",  # core + ecs foundation packages
    "ses_edee7d6b9ffe9APXgCKvQg3Mns",  # content loader, validation, game data
    "ses_edee7d6b8ffesUw1bSWVkTTrFn",  # sim engine systems + tests
    "ses_edee7d6afffelbp0lv7CNpLGoQ",  # render, ui, game client layer
    "ses_eddde25b7ffeyH1HYAUNFVpXZz",  # sim: NPC behavior, visibility, ammo
    "ses_eddde25b5ffe9d4df4mnC8YOJR",  # client: UI scale, windows, scanner
    "ses_edea532e2ffeystgPcwWIvEWnl",  # sim polish: transfer, respawn, save cleanup
    "ses_edaf36b75ffeLtArM5Hy0tmlYU",  # ebitenui spike: theme + resizable overview
    "ses_eda8e298effekHkeNKvFTBfd2M",  # uikit: table widget + data windows
    "ses_eda8e298cffeNyS7HDmpcllZph",  # uikit: HUD, rack, log, scanner, starmap, help
    "ses_eda31b252ffeh4EczBaF9acnQI",  # sim: timed warp profile
    "ses_eda30aa8affeoANRHdDXaaQeji",  # game: flip to uikit UI layer
    "ses_ed9e688a1ffegXvwWcQVkvlYB0",  # game: adopt uikit hooks (spawn blocked)
    "ses_ed9c9ecf1ffeiZV1zVXBv2C0Gb",  # game: adopt uikit hooks (retry)
]


def api_json(path: str):
    """Fetch an API path as JSON via the OpenCode CLI (file-mediated so large
    payloads are never truncated by any pipe buffer)."""
    tmp = Path("/tmp/opencode/_api_payload.json")
    tmp.parent.mkdir(parents=True, exist_ok=True)
    with tmp.open("wb") as fh:
        subprocess.run(["opencode", "api", "get", path], stdout=fh, check=True)
    return json.loads(tmp.read_text())


def slugify(title: str) -> str:
    s = re.sub(r"[^a-z0-9]+", "-", (title or "untitled").lower()).strip("-")
    return s[:60] or "untitled"


def ts_ms(ms) -> str:
    if not ms:
        return ""
    return datetime.datetime.fromtimestamp(ms / 1000).isoformat(timespec="seconds")


def render_message(msg: dict, out: list) -> dict:
    """Append one message as Markdown; return block-type counts."""
    counts = {}
    role = msg.get("role") or msg.get("type") or "message"
    meta = []
    if msg.get("agent"):
        meta.append(f"agent: `{msg['agent']}`")
    if msg.get("model"):
        m = msg["model"]
        if isinstance(m, dict) and m.get("id"):
            meta.append(f"model: `{m.get('providerID', '?')}/{m['id']}`")
    if msg.get("time", {}).get("created"):
        meta.append(ts_ms(msg["time"]["created"]))
    out.append(f"\n## [{role}] {' · '.join(meta)}\n")

    blocks = msg.get("content") or msg.get("parts") or []
    for b in blocks:
        t = b.get("type", "unknown")
        counts[t] = counts.get(t, 0) + 1
        if t == "reasoning":
            out.append("\n### 💭 Thinking\n")
            out.append("\n" + (b.get("text") or "") + "\n")
        elif t == "text":
            out.append("\n" + (b.get("text") or "") + "\n")
        elif t == "tool":
            name = b.get("name") or b.get("tool") or "tool"
            out.append(f"\n### 🔧 Tool call: `{name}`\n")
            out.append("\n```json\n" + json.dumps(b, indent=2, ensure_ascii=False, default=str) + "\n```\n")
        else:
            out.append(f"\n### `{t}`\n")
            out.append("\n```json\n" + json.dumps(b, indent=2, ensure_ascii=False, default=str) + "\n```\n")
    return counts


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="docs/subagent-transcripts")
    args = ap.parse_args()

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    # Session metadata (titles) for the index — metadata only.
    listing = api_json("/api/session")
    items = listing if isinstance(listing, list) else (listing.get("data") or listing.get("sessions") or [])
    titles = {}
    for s in items:
        if isinstance(s, dict) and s.get("id"):
            titles[s["id"]] = s.get("title") or ""

    index_rows = []
    total_blocks = 0
    for sid in SESSION_IDS:
        try:
            payload = api_json(f"/api/session/{sid}/message")
        except Exception as exc:  # noqa: BLE001 - report and continue
            print(f"! {sid}: fetch failed ({type(exc).__name__})", file=sys.stderr)
            continue
        messages = payload.get("data") if isinstance(payload, dict) else payload
        messages = messages or []

        out = [
            f"# Subagent conversation — {titles.get(sid, 'untitled')}\n",
            "\nArchived verbatim from the local OpenCode service "
            "(`opencode api get /api/session/{id}/message`), including "
            "reasoning/thought blocks.\n",
            f"\n- Session: `{sid}`\n- Title: {titles.get(sid, 'untitled')}\n"
            f"- Messages: {len(messages)}\n",
        ]
        counts_all = {}
        for msg in messages:
            for k, v in render_message(msg, out).items():
                counts_all[k] = counts_all.get(k, 0) + v
        total_blocks += sum(counts_all.values())
        out.append(
            "\n---\n\n_Block totals: "
            + ", ".join(f"{k}={v}" for k, v in sorted(counts_all.items()))
            + "_\n"
        )

        fname = f"{slugify(titles.get(sid, sid))}--{sid[-8:]}.md"
        path = out_dir / fname
        path.write_text("".join(out), encoding="utf-8")
        index_rows.append((fname, sid, titles.get(sid, ""), len(messages), counts_all, path.stat().st_size))

    idx = [
        "# Subagent transcripts\n",
        "\nVerbatim conversations (including thoughts) of the subagents that "
        "built Entieve, archived from the local OpenCode service. Regenerate "
        "with `python3 scripts/extract_subagent_transcripts.py`.\n",
        "\n| File | Session | Title | Messages | Blocks | Bytes |\n|---|---|---|---|---|---|\n",
    ]
    for fname, sid, title, nmsg, counts, size in index_rows:
        blocks = ", ".join(f"{k}={v}" for k, v in sorted(counts.items()))
        idx.append(f"| `{fname}` | `{sid}` | {title} | {nmsg} | {blocks} | {size} |\n")
    (out_dir / "README.md").write_text("".join(idx), encoding="utf-8")

    # Metadata report only — never the content.
    print(f"sessions extracted: {len(index_rows)}/{len(SESSION_IDS)}")
    print(f"total content blocks archived: {total_blocks}")
    for fname, sid, _title, nmsg, counts, size in index_rows:
        print(f"  {fname}  msgs={nmsg}  blocks={sum(counts.values())}  bytes={size}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
