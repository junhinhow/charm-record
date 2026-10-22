package record

import (
	"fmt"
	"os"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Recorder envolve um modelo e grava todas as mensagens que passam pelo
// Update. Ele não executa comandos nem desenha em um terminal: é um adaptador
// para uso direto em testes.
//
// Um Recorder não é seguro para uso concorrente.
type Recorder struct {
	model  tea.Model
	width  int
	height int
	meta   map[string]string
	events []Event
	lost   int
	start  time.Time
}

// NewRecorder cria um Recorder para model com o tamanho inicial informado.
// Largura ou altura negativas são tratadas como zero. model pode ser nil; nesse
// caso Update e View são no-ops, mas o cassete continua sendo montado.
func NewRecorder(model tea.Model, width, height int) *Recorder {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return &Recorder{
		model:  model,
		width:  width,
		height: height,
		meta:   map[string]string{},
		start:  time.Now(),
	}
}

// Update grava a mensagem e a repassa ao modelo. O comando retornado é
// encaminhado ao chamador, mas não é registrado no cassete.
func (r *Recorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r == nil {
		return nil, nil
	}

	typ, data, lost := encodeMsg(msg)
	if lost {
		r.lost++
		r.meta[MetaUnserializable] = strconv.Itoa(r.lost)
	}
	at := time.Since(r.start).Milliseconds()
	if at < 0 {
		at = 0
	}
	r.events = append(r.events, Event{Type: typ, Data: data, At: at})

	if r.model == nil {
		return nil, nil
	}
	next, cmd := r.model.Update(msg)
	if next != nil {
		r.model = next
	}
	return r.model, cmd
}

// View delega o desenho ao modelo atual.
func (r *Recorder) View() tea.View {
	if r == nil || r.model == nil {
		return tea.View{}
	}
	return r.model.View()
}

// Cassette devolve um instantâneo do cassete gravado até agora. O resultado é
// uma cópia: alterar o retorno não modifica o Recorder.
func (r *Recorder) Cassette() *Cassette {
	if r == nil {
		return nil
	}
	c := &Cassette{
		Version: Version,
		Width:   r.width,
		Height:  r.height,
		Meta:    make(map[string]string, len(r.meta)),
		Events:  append([]Event{}, r.events...),
	}
	for k, v := range r.meta {
		c.Meta[k] = v
	}
	return c
}

// AddMeta associa um par chave/valor ao cassete. Chaves são serializadas em
// ordem alfabética, então a ordem das chamadas não altera os bytes salvos.
func (r *Recorder) AddMeta(k, v string) {
	if r == nil {
		return
	}
	if r.meta == nil {
		r.meta = map[string]string{}
	}
	r.meta[k] = v
}

// Save grava o cassete em path como JSON indentado. O mesmo cassete salvo duas
// vezes produz bytes idênticos.
func (r *Recorder) Save(path string) error {
	if r == nil {
		return fmt.Errorf("record: salvar cassete %q: %w", path, ErrNilCassette)
	}
	data, err := r.Cassette().marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("record: salvar cassete %q: %w", path, err)
	}
	return nil
}

// Reset limpa eventos, metadados e contadores e reinicia o relógio relativo.
// O modelo e as dimensões são preservados.
func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.events = nil
	r.meta = map[string]string{}
	r.lost = 0
	r.start = time.Now()
}
