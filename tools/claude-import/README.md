# Claude transcript import

This offline converter writes native OMO v3 sessions. It does not run OMO,
contact a provider, execute tools, or modify source transcripts.

```sh
go test -race -shuffle=on -count=1 ./tools/claude-import
go build -buildvcs=false -o /tmp/claude-import ./tools/claude-import
sudo -u ujawhales /tmp/claude-import \
  -root /home/ujawhales/Claude-Cowork \
  -sessions /home/ujawhales/.omo/agent/sessions
```

Use `-dry-run` to scan and validate without writing. Run as the target account;
directories are created with mode 0700 and files with mode 0600.

Discovery examines at most the first three lines of each `.jsonl` under the root.
Only selected transcript files are subsequently read in full. Files containing Claude
user/assistant records with a matching `message.role`, `uuid`, and `parentUuid`
in that probe are imported. A Claude system record with `uuid`, `parentUuid`
and `compactMetadata` also qualifies, covering compacted main-session exports.
Other data JSONL files are not sessions. Malformed JSON in a
recognized transcript fails the import rather than silently dropping messages.
The first directory under the root is the project cwd; the original cwd values,
source path, and source SHA-256 are retained as metadata.

The source path's SHA-256 determines a stable session UUID and filename. Source
content has its own SHA-256 in the header and receipt. Reimports skip existing
destinations, including changed-source snapshots, so a resumed session is never
overwritten and a changed source cannot create duplicates. To deliberately
replace an imported snapshot, first archive its destination outside the sessions
directory. Receipts contain paths, hashes, counts, and generated names, not
transcript bodies.

Parent references are topologically ordered and preserved; split records attach
children to their final entry. Missing parents (e.g. compacted-away history)
become roots, with original references retained in the source. Compaction
summaries and attachments become labelled context notes. Unsupported binary
blocks become text notes, not base64 payloads. Historical tool calls retain their
IDs and arguments; results resolve names along their own ancestry. Orphan results
become notes rather than invalid tool-result messages. This is a history import,
not a tool replay; unfinished historical calls are not executed by this program.

Subagents sharing a Claude session ID with a main transcript in the same project
become separate sessions. Their names identify the parent and agent filename,
and their headers include the native `parentSession` path.

## Webchat integration

The webchat's allowed root must contain the imported project cwd. A service
restricted to `/home/ujawhales/projects` cannot register a workspace under
`/home/ujawhales/Claude-Cowork`; do not change the session cwd to work around
that boundary.

On webchat versions where disk sessions only contribute recency to enrolled
chats, native OMO discovery alone does not enroll imports in the webchat catalog.
Use the workspace session-adoption API after registering the correct workspace,
or integrate disk discovery in the orchestrator's history feature. This converter
does not bypass the API or modify the webchat's private cursor store.
