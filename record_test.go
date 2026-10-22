package record_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	record "github.com/junhinhow/charm-record"
)

// addMsg é uma mensagem própria do teste, usada para exercitar o registro de
// tipos e o roundtrip JSON.
type addMsg struct {
	Text string `json:"text"`
}

func init() { record.RegisterMsg(addMsg{}) }

type testModel struct {
	text    string
	width   int
	height  int
	updates int
}

func newTestModel() testModel { return testModel{} }

func (m testModel) Init() tea.Cmd { return nil }

func (m testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updates++
	switch msg := msg.(type) {
	case addMsg:
		m.text += msg.Text
	case tea.KeyPressMsg:
		m.text += msg.String()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	}
	return m, nil
}

func (m testModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("text=%s %dx%d updates=%d", m.text, m.width, m.height, m.updates))
}

func TestGravaTresMensagensEReproduz(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	for _, text := range []string{"a", "b", "c"} {
		rec.Update(addMsg{Text: text})
	}

	c := rec.Cassette()
	if got, want := len(c.Events), 3; got != want {
		t.Fatalf("eventos gravados = %d, quer %d", got, want)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	frames := rep.Frames()
	if got, want := len(frames), 3; got != want {
		t.Fatalf("frames = %d, quer %d", got, want)
	}
	if frames[0] == frames[2] {
		t.Fatalf("frames não refletem a evolução do modelo: %q", frames)
	}
	if got, want := rep.View().Content, rec.View().Content; got != want {
		t.Fatalf("View final = %q, quer %q", got, want)
	}
}

func TestSaveDeterministico(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.AddMeta("zeta", "último")
	rec.AddMeta("alpha", "primeiro")
	rec.AddMeta("mike", "meio")
	rec.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rec.Update(addMsg{Text: "x"})

	path := filepath.Join(t.TempDir(), "sessao.json")
	if err := rec.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !json.Valid(first) {
		t.Fatalf("JSON inválido: %s", first)
	}

	for i := 0; i < 20; i++ {
		if err := rec.Save(path); err != nil {
			t.Fatalf("Save iteração %d: %v", i, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile iteração %d: %v", i, err)
		}
		if !bytes.Equal(first, got) {
			t.Fatalf("bytes diferentes na iteração %d\nprimeiro:\n%s\nagora:\n%s", i, first, got)
		}
	}

	text := string(first)
	if strings.Index(text, `"alpha"`) > strings.Index(text, `"zeta"`) {
		t.Fatalf("chaves de Meta fora de ordem:\n%s", text)
	}
}

func TestRoundTrip(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 90, 25)
	rec.AddMeta("app", "demo")
	rec.Update(tea.WindowSizeMsg{Width: 90, Height: 25})
	rec.Update(addMsg{Text: "olá"})
	rec.Update(tea.KeyPressMsg{Code: '!', Text: "!"})

	path := filepath.Join(t.TempDir(), "sessao.json")
	if err := rec.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c, err := record.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Version != record.Version {
		t.Fatalf("Version = %d, quer %d", c.Version, record.Version)
	}
	if c.Width != 90 || c.Height != 25 {
		t.Fatalf("dimensões = %dx%d, quer 90x25", c.Width, c.Height)
	}
	if got, want := c.Meta["app"], "demo"; got != want {
		t.Fatalf("Meta[app] = %q, quer %q", got, want)
	}
	if got, want := len(c.Events), 3; got != want {
		t.Fatalf("eventos = %d, quer %d", got, want)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := rep.View().Content, rec.View().Content; got != want {
		t.Fatalf("View final = %q, quer %q", got, want)
	}
}

func TestMetaDeterministicaComMuitasChaves(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 1, 1)
	keys := []string{"k09", "k01", "k05", "k00", "k19", "k02", "k11", "k07", "k03", "k15"}
	for _, k := range keys {
		rec.AddMeta(k, "v-"+k)
	}
	rec.Update(addMsg{Text: "a"})

	path := filepath.Join(t.TempDir(), "meta.json")
	if err := rec.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	for i := 0; i < 20; i++ {
		if err := rec.Save(path); err != nil {
			t.Fatalf("Save iteração %d: %v", i, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile iteração %d: %v", i, err)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("ordem de Meta mudou na iteração %d", i)
		}
	}
}

func TestReset(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.AddMeta("a", "b")
	rec.Update(addMsg{Text: "a"})
	rec.Update(addMsg{Text: "b"})

	rec.Reset()
	c := rec.Cassette()
	if got := len(c.Events); got != 0 {
		t.Fatalf("eventos após Reset = %d, quer 0", got)
	}
	if got := len(c.Meta); got != 0 {
		t.Fatalf("meta após Reset = %d entradas, quer 0", got)
	}

	rec.Update(addMsg{Text: "c"})
	c = rec.Cassette()
	if got, want := len(c.Events), 1; got != want {
		t.Fatalf("eventos após nova gravação = %d, quer %d", got, want)
	}
	if c.Events[0].At < 0 {
		t.Fatalf("At negativo: %d", c.Events[0].At)
	}
}

func TestStepAvancaUmPorVez(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(addMsg{Text: "a"})
	rec.Update(addMsg{Text: "b"})
	rec.Update(addMsg{Text: "c"})

	// At enorme não deve causar espera: o replay ignora o relógio.
	c := rec.Cassette()
	for i := range c.Events {
		c.Events[i].At = 3_600_000
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}

	for i, want := range []string{"a", "b", "c"} {
		msg, err := rep.Step()
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		got, ok := msg.(addMsg)
		if !ok {
			t.Fatalf("Step %d devolveu %T, quer addMsg", i, msg)
		}
		if got.Text != want {
			t.Fatalf("Step %d = %q, quer %q", i, got.Text, want)
		}
		if n := len(rep.Frames()); n != i+1 {
			t.Fatalf("frames após Step %d = %d, quer %d", i, n, i+1)
		}
	}

	if _, err := rep.Step(); !errors.Is(err, io.EOF) {
		t.Fatalf("Step no fim = %v, quer io.EOF", err)
	}
	if got, want := rep.View().Content, rec.View().Content; got != want {
		t.Fatalf("View final = %q, quer %q", got, want)
	}
}

func TestCassetteNaoCompartilhaEstado(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 10, 20)
	rec.AddMeta("a", "1")
	rec.Update(addMsg{Text: "x"})

	c := rec.Cassette()
	c.Meta["a"] = "alterado"
	c.Events[0].Type = "alterado"

	fresh := rec.Cassette()
	if got := fresh.Meta["a"]; got != "1" {
		t.Fatalf("Meta compartilhada: %q", got)
	}
	if got := fresh.Events[0].Type; got != "msg" {
		t.Fatalf("Events compartilhados: %q", got)
	}
}

func TestNilNaoEntraEmPanic(t *testing.T) {
	rec := record.NewRecorder(nil, -10, -20)
	if c := rec.Cassette(); c.Width != 0 || c.Height != 0 {
		t.Fatalf("dimensões negativas não foram normalizadas: %dx%d", c.Width, c.Height)
	}
	if _, cmd := rec.Update(addMsg{Text: "x"}); cmd != nil {
		t.Fatalf("Update com modelo nil devolveu comando não nil")
	}
	if got := rec.View().Content; got != "" {
		t.Fatalf("View com modelo nil = %q, quer vazia", got)
	}
	if err := rec.Save(filepath.Join(t.TempDir(), "vazio.json")); err != nil {
		t.Fatalf("Save com modelo nil: %v", err)
	}

	if _, err := record.NewReplayer(nil, rec.Cassette()); !errors.Is(err, record.ErrNilModel) {
		t.Fatalf("NewReplayer(model nil) = %v, quer ErrNilModel", err)
	}
	if _, err := record.NewReplayer(newTestModel(), nil); !errors.Is(err, record.ErrNilCassette) {
		t.Fatalf("NewReplayer(cassette nil) = %v, quer ErrNilCassette", err)
	}

	var nilRec *record.Recorder
	nilRec.Update(addMsg{Text: "x"})
	_ = nilRec.View()
	if err := nilRec.Save("x.json"); err == nil {
		t.Fatal("Save em Recorder nil deveria falhar")
	}

	var nilRep *record.Replayer
	_ = nilRep.View()
	_ = nilRep.Model()
	if got := nilRep.Frames(); got != nil {
		t.Fatalf("Frames de Replayer nil = %v, quer nil", got)
	}
	if _, err := nilRep.Step(); err == nil {
		t.Fatal("Step de Replayer nil deveria falhar")
	}
}

func TestLoadErros(t *testing.T) {
	_, err := record.Load(filepath.Join(t.TempDir(), "nao-existe.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load de arquivo inexistente = %v, quer fs.ErrNotExist", err)
	}

	_, err = record.LoadBytes([]byte("{isso não é json"))
	if err == nil {
		t.Fatal("LoadBytes com JSON inválido deveria falhar")
	}
	if !strings.Contains(err.Error(), "decodificar cassete") {
		t.Fatalf("erro pouco claro: %v", err)
	}
}

func TestVersaoErrada(t *testing.T) {
	_, err := record.LoadBytes([]byte(`{"version": 2, "events": []}`))
	if !errors.Is(err, record.ErrVersionMismatch) {
		t.Fatalf("erro = %v, quer ErrVersionMismatch", err)
	}
	var ve *record.VersionError
	if !errors.As(err, &ve) {
		t.Fatalf("erro não é *VersionError: %T", err)
	}
	if ve.Found != 2 || ve.Expected != record.Version {
		t.Fatalf("VersionError = %+v", ve)
	}
	if !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), strconv.Itoa(record.Version)) {
		t.Fatalf("mensagem não informa as versões: %v", err)
	}

	_, err = record.NewReplayer(newTestModel(), &record.Cassette{Version: 99})
	if !errors.Is(err, record.ErrVersionMismatch) {
		t.Fatalf("NewReplayer com versão errada = %v, quer ErrVersionMismatch", err)
	}
}
