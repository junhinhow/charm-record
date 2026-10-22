package record_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	record "github.com/junhinhow/charm-record"
)

type badMsg struct {
	Ch chan int
}

// autoMsg não é registrada explicitamente em lugar nenhum: o Recorder deve
// registrar o tipo automaticamente para o replay em memória.
type autoMsg struct {
	N int `json:"n"`
}

type autoModel struct {
	n int
}

func (m autoModel) Init() tea.Cmd { return nil }

func (m autoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if got, ok := msg.(autoMsg); ok {
		m.n = got.N
	}
	return m, nil
}

func (m autoModel) View() tea.View { return tea.NewView(fmt.Sprintf("n=%d", m.n)) }

func TestRegistroAutomaticoNoProcesso(t *testing.T) {
	rec := record.NewRecorder(autoModel{}, 80, 24)
	rec.Update(autoMsg{N: 7})

	rep, err := record.NewReplayer(autoModel{}, rec.Cassette())
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := rep.View().Content, "n=7"; got != want {
		t.Fatalf("View = %q, quer %q", got, want)
	}
	if got := len(rep.Warnings()); got != 0 {
		t.Fatalf("avisos = %v, quer nenhum", rep.Warnings())
	}
}

func TestTeclaPreservada(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	c := rec.Cassette()
	if got, want := c.Events[0].Type, "key"; got != want {
		t.Fatalf("Type = %q, quer %q", got, want)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(c.Events[0].Data), &payload); err != nil {
		t.Fatalf("payload de tecla inválido: %v", err)
	}
	if got := payload["code"]; got != float64('c') {
		t.Fatalf("code = %v, quer %q", got, 'c')
	}
	if got := payload["mod"]; got != float64(tea.ModCtrl) {
		t.Fatalf("mod = %v, quer %d", got, tea.ModCtrl)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := rep.View().Content; !strings.Contains(got, "text=ctrl+c") {
		t.Fatalf("View = %q, não contém a tecla ctrl+c", got)
	}
}

func TestTeclaReleasePreservada(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(tea.KeyReleaseMsg{Code: 'a', Text: "a"})

	rep, err := record.NewReplayer(newTestModel(), rec.Cassette())
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	msg, err := rep.Step()
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	release, ok := msg.(tea.KeyReleaseMsg)
	if !ok {
		t.Fatalf("mensagem = %T, quer tea.KeyReleaseMsg", msg)
	}
	if release.Code != 'a' || release.Text != "a" {
		t.Fatalf("tecla = %+v", release)
	}
}

func TestResizePreservado(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	c := rec.Cassette()
	if got, want := c.Events[0].Type, "resize"; got != want {
		t.Fatalf("Type = %q, quer %q", got, want)
	}
	if c.Width != 80 || c.Height != 24 {
		t.Fatalf("dimensões iniciais = %dx%d, quer 80x24", c.Width, c.Height)
	}

	var payload map[string]int
	if err := json.Unmarshal([]byte(c.Events[0].Data), &payload); err != nil {
		t.Fatalf("payload de resize inválido: %v", err)
	}
	if payload["width"] != 120 || payload["height"] != 40 {
		t.Fatalf("payload = %v, quer 120x40", payload)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	model, ok := rep.Model().(testModel)
	if !ok {
		t.Fatalf("modelo = %T, quer testModel", rep.Model())
	}
	if model.width != 120 || model.height != 40 {
		t.Fatalf("modelo = %dx%d, quer 120x40", model.width, model.height)
	}
}

func TestMensagemNaoSerializavel(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(badMsg{Ch: make(chan int)})
	rec.Update(addMsg{Text: "a"})

	c := rec.Cassette()
	if got := c.Meta[record.MetaUnserializable]; got != "1" {
		t.Fatalf("Meta[%s] = %q, quer %q", record.MetaUnserializable, got, "1")
	}
	if got := len(c.Events); got != 2 {
		t.Fatalf("eventos = %d, quer 2", got)
	}
	if got, want := c.Events[0].Type, "msg"; got != want {
		t.Fatalf("Type = %q, quer %q", got, want)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(c.Events[0].Data), &payload); err != nil {
		t.Fatalf("payload inválido: %v", err)
	}
	if name := payload["__unserializable"]; !strings.Contains(name, "badMsg") {
		t.Fatalf("__unserializable = %q, quer o nome do tipo", name)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	warnings := rep.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("avisos = %v, quer 1", warnings)
	}
	if !strings.Contains(warnings[0], "não serializável") {
		t.Fatalf("aviso = %q, quer menção a não serializável", warnings[0])
	}
	if got, want := len(rep.Frames()), 2; got != want {
		t.Fatalf("frames = %d, quer %d", got, want)
	}
	if got := rep.View().Content; !strings.Contains(got, "text=a") {
		t.Fatalf("View = %q, o replay deveria continuar após o evento ignorado", got)
	}
}

func TestTipoDesconhecidoNoReplay(t *testing.T) {
	c := &record.Cassette{
		Version: record.Version,
		Events: []record.Event{{
			Type: "msg",
			Data: `{"__type":"exemplo.tipoNaoRegistrado","__data":{"x":1}}`,
		}},
	}
	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	warnings := rep.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "não registrado") {
		t.Fatalf("avisos = %v, quer aviso de tipo não registrado", warnings)
	}
	if got, want := len(rep.Frames()), 1; got != want {
		t.Fatalf("frames = %d, quer %d", got, want)
	}
}

func TestEventoCmdIgnoradoComAviso(t *testing.T) {
	c := &record.Cassette{
		Version: record.Version,
		Events:  []record.Event{{Type: "cmd", Data: `{"name":"tick"}`}},
	}
	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	msg, err := rep.Step()
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if msg != nil {
		t.Fatalf("Step devolveu %T, quer nil", msg)
	}
	warnings := rep.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "tick") {
		t.Fatalf("avisos = %v, quer menção ao comando tick", warnings)
	}
}

func TestMensagemNil(t *testing.T) {
	rec := record.NewRecorder(newTestModel(), 80, 24)
	rec.Update(nil)

	c := rec.Cassette()
	var payload map[string]any
	if err := json.Unmarshal([]byte(c.Events[0].Data), &payload); err != nil {
		t.Fatalf("payload inválido: %v", err)
	}
	if got := payload["__type"]; got != "<nil>" {
		t.Fatalf("__type = %v, quer <nil>", got)
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	msg, err := rep.Step()
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if msg != nil {
		t.Fatalf("mensagem = %T, quer nil", msg)
	}
	if got := len(rep.Warnings()); got != 0 {
		t.Fatalf("avisos = %v, quer nenhum", rep.Warnings())
	}
}

func TestCasseteVazio(t *testing.T) {
	c, err := record.LoadBytes([]byte(`{"version": 1, "events": []}`))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if c.Meta == nil || c.Events == nil {
		t.Fatal("Meta e Events deveriam ser coleções vazias após Load")
	}

	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	if err := rep.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(rep.Frames()); got != 0 {
		t.Fatalf("frames = %d, quer 0", got)
	}

	rec := record.NewRecorder(newTestModel(), 80, 24)
	path := t.TempDir() + "/vazio.json"
	if err := rec.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c, err = record.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(c.Events); got != 0 {
		t.Fatalf("eventos = %d, quer 0", got)
	}
}

func TestEventoDesconhecido(t *testing.T) {
	c := &record.Cassette{
		Version: record.Version,
		Events:  []record.Event{{Type: "voar", Data: `{}`}},
	}
	rep, err := record.NewReplayer(newTestModel(), c)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	_, err = rep.Step()
	if !errors.Is(err, record.ErrUnknownEvent) {
		t.Fatalf("erro = %v, quer ErrUnknownEvent", err)
	}
	if !strings.Contains(err.Error(), "evento 0") {
		t.Fatalf("erro não informa o índice: %v", err)
	}
	if !strings.Contains(err.Error(), "voar") {
		t.Fatalf("erro não informa o tipo: %v", err)
	}
	if got := len(rep.Frames()); got != 0 {
		t.Fatalf("frames = %d, quer 0 após erro", got)
	}
}
