package record

import (
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

// Replayer aplica os eventos de um cassete a um modelo, sem terminal e sem
// timers. O campo [Event.At] é apenas informativo e nunca é usado para
// esperar: a reprodução é síncrona e determinística.
//
// O Replayer não chama [tea.Model.Init] e ignora os comandos retornados pelo
// Update. O modelo deve chegar pronto para receber as mensagens gravadas.
// Cada Replayer deve ser usado uma única vez.
type Replayer struct {
	model    tea.Model
	events   []Event
	index    int
	frames   []string
	warnings []string
}

// NewReplayer valida o cassete e prepara a reprodução sobre model.
func NewReplayer(model tea.Model, c *Cassette) (*Replayer, error) {
	if model == nil {
		return nil, fmt.Errorf("record: criar replayer: %w", ErrNilModel)
	}
	if c == nil {
		return nil, fmt.Errorf("record: criar replayer: %w", ErrNilCassette)
	}
	if c.Version != Version {
		return nil, &VersionError{Found: c.Version, Expected: Version}
	}
	return &Replayer{
		model:  model,
		events: append([]Event{}, c.Events...),
	}, nil
}

// Run aplica todos os eventos restantes. Eventos pulados geram avisos, mas não
// interrompem a execução. Retorna nil ao chegar ao fim.
func (r *Replayer) Run() error {
	for {
		if _, err := r.Step(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// Step aplica exatamente um evento e devolve a mensagem entregue ao modelo.
// Eventos ignorados (não serializáveis, de tipo não registrado ou "cmd")
// avançam o índice, acrescentam um frame e retornam (nil, nil), com o motivo em
// [Replayer.Warnings].
//
// Ao fim dos eventos, Step retorna (nil, [io.EOF]).
func (r *Replayer) Step() (tea.Msg, error) {
	if r == nil {
		return nil, fmt.Errorf("record: replayer nil: %w", ErrNilCassette)
	}
	if r.index >= len(r.events) {
		return nil, io.EOF
	}

	i := r.index
	ev := r.events[i]
	msg, warning, err := decodeEvent(ev)
	if err != nil {
		return nil, fmt.Errorf("record: evento %d (%s): %w", i, ev.Type, err)
	}

	r.index++
	if warning != "" {
		r.warnings = append(r.warnings, fmt.Sprintf("evento %d: %s", i, warning))
		r.frames = append(r.frames, r.View().Content)
		return nil, nil
	}

	next, _ := r.model.Update(msg)
	if next != nil {
		r.model = next
	}
	r.frames = append(r.frames, r.View().Content)
	return msg, nil
}

// View devolve a View do modelo no estado atual.
func (r *Replayer) View() tea.View {
	if r == nil || r.model == nil {
		return tea.View{}
	}
	return r.model.View()
}

// Model devolve o modelo no estado atual.
func (r *Replayer) Model() tea.Model {
	if r == nil {
		return nil
	}
	return r.model
}

// Frames devolve a View (seu conteúdo textual) após cada evento processado, na
// ordem. Eventos ignorados também geram um frame, de modo que o comprimento é
// igual ao número de eventos já processados.
func (r *Replayer) Frames() []string {
	if r == nil {
		return nil
	}
	return append([]string{}, r.frames...)
}

// Warnings devolve os avisos acumulados durante a reprodução. Eles indicam
// eventos que não puderam ser aplicados, mas não invalidam o cassete.
func (r *Replayer) Warnings() []string {
	if r == nil {
		return nil
	}
	return append([]string{}, r.warnings...)
}
