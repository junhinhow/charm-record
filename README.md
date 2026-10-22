# charm-record

Record and replay Bubble Tea TUI sessions as deterministic JSON cassettes for tests.

> [Leia em Português](README.pt-br.md)

## Install

```bash
go get github.com/junhinhow/charm-record
```

## Usage

A minimal model:

```go
type Model struct{ Count int }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "+" {
		m.Count++
	}
	return m, nil
}

func (m Model) View() tea.View {
	return tea.NewView(fmt.Sprintf("count=%d", m.Count))
}
```

### Record a session in a test

```go
func TestRecordSession(t *testing.T) {
	rec := record.NewRecorder(Model{}, 80, 24)
	rec.AddMeta("scenario", "increment")

	rec.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})

	if err := rec.Save("testdata/increment.cassette.json"); err != nil {
		t.Fatal(err)
	}
}
```

The cassette is plain, indented JSON. `At` is the only time-related field: it
stores milliseconds relative to the start of the recording.

### Replay in CI

```go
func TestReplaySession(t *testing.T) {
	c, err := record.Load("testdata/increment.cassette.json")
	if err != nil {
		t.Fatal(err)
	}

	rep, err := record.NewReplayer(Model{}, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Run(); err != nil {
		t.Fatal(err)
	}

	frames := rep.Frames() // one view per event, in order
	if got, want := rep.View().Content, "count=2"; got != want {
		t.Fatalf("final view = %q, want %q", got, want)
	}
	_ = frames // feed them to golden snapshots if you like
}
```

No TTY, no `time.Sleep`, and no real timers are involved: the same cassette
always produces the same frames.

### Custom messages

`WindowSizeMsg` becomes a `resize` event and key messages become `key` events.
Any other message is stored as `msg` with its Go type name and JSON payload.
When you record, the type is registered automatically in the current process,
so recording and replaying in the same test needs no extra setup. To replay a
cassette loaded in another process (for example, in CI), register a prototype
once:

```go
type loadedMsg struct{ Items []string }

func init() { record.RegisterMsg(loadedMsg{}) }
```

The registry is keyed by `reflect.Type.String()` (for example
`counter.loadedMsg`). Register the same package/type name in the replay binary.

## API

| Symbol | Description |
| --- | --- |
| `Version` | Current cassette format version (`1`). |
| `Event` | One session step: `Type`, `Data` (JSON string), `At` (relative ms). |
| `Cassette` | Recorded session: version, initial size, metadata, events. |
| `NewRecorder` | Wraps a model and starts recording. |
| `Recorder.Update` | Records a message and forwards it to the model. |
| `Recorder.View` | Current view from the wrapped model. |
| `Recorder.Cassette` | Snapshot copy of the cassette. |
| `Recorder.AddMeta` | Adds a metadata key/value pair. |
| `Recorder.Save` | Writes deterministic indented JSON. |
| `Recorder.Reset` | Clears events, metadata, and counters. |
| `Load`, `LoadBytes` | Read and validate a cassette. |
| `RegisterMsg` | Registers a custom message prototype for replay. |
| `NewReplayer` | Validates a cassette and prepares replay. |
| `Replayer.Run` | Applies all remaining events. |
| `Replayer.Step` | Applies exactly one event; returns `io.EOF` when done. |
| `Replayer.View` / `Model` | Current view / model. |
| `Replayer.Frames` | View content after each processed event. |
| `Replayer.Warnings` | Events that were skipped, with reasons. |
| `MetaUnserializable` | Metadata key with the number of unserializable messages. |
| `ErrVersionMismatch`, `VersionError` | Cassette format version mismatch. |
| `ErrNilModel`, `ErrNilCassette`, `ErrUnknownEvent` | Typed errors for edge cases. |

## Design notes

- **Deterministic output.** JSON is indented with two spaces and map keys are
  sorted. Saving the same cassette twice produces identical bytes. Only `At` is
  temporal, and it is relative, never an absolute timestamp.
- **Versioned format.** Any version other than `Version` fails with a
  `*VersionError` that reports both the found and expected versions;
  `errors.Is(err, ErrVersionMismatch)` works.
- **Unserializable messages.** If `json.Marshal` fails, the event is stored as
  `{"__unserializable": "<type>"}` and counted in `Meta` under
  `MetaUnserializable`. Replay skips it, appends a warning, and continues. A
  registered type missing from the binary is handled the same way.
- **Commands are not recorded.** A `tea.Cmd` is a function, not data, so
  `Update` forwards it to the caller but does not execute or store it. The
  `cmd` event type is recognized for external tooling and skipped with a
  warning during replay.
- **No waiting.** `At` is informational. Replay is synchronous and ignores it.
- **Frames.** There is one frame per processed event, including skipped events;
  `Frames` returns `tea.View.Content` strings for golden comparisons.
- **Single use.** `Init` is not called, returned commands are ignored, and a
  `Replayer` is meant to be run once. Start from a fresh model. `Recorder` and
  `Replayer` are not safe for concurrent use; the message registry is.
- **Initial size is metadata.** `Width` and `Height` describe the session, but
  replay does not synthesize a `WindowSizeMsg`. Record one if the model needs
  it; real Bubble Tea programs receive one at startup.
- **Empty cassettes are valid.** A cassette with no events loads and replays as
  a no-op.

## License

MIT
