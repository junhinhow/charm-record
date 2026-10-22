package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// RegisterMsg registra um protótipo para que mensagens desse tipo possam ser
// reconstruídas durante o replay.
//
// O protótipo pode ser um valor (minhaMsg{}) ou um ponteiro ((*minhaMsg)(nil)).
// O nome usado no cassete é o nome do tipo Go, como aparece em
// reflect.Type.String(); por isso o registro precisa acontecer no mesmo pacote
// ou em um pacote com o mesmo caminho de importação usado na gravação.
//
// O [Recorder] registra automaticamente, no processo atual, o tipo de toda
// mensagem serializável que grava. RegisterMsg é necessário para reproduzir um
// cassete carregado em outro processo, como um teste de CI.
//
// Chamar RegisterMsg com nil é uma operação sem efeito. O registro é global e
// seguro para uso concorrente.
func RegisterMsg(prototype any) {
	t := reflect.TypeOf(prototype)
	if t == nil {
		return
	}
	registryMu.Lock()
	msgTypes[t.String()] = t
	registryMu.Unlock()
}

var (
	registryMu sync.RWMutex
	msgTypes   = map[string]reflect.Type{}
)

// errTypeNotRegistered é interno: o replay transforma esse caso em aviso, não
// em erro fatal, porque o cassete é válido mesmo sem o tipo no binário atual.
var errTypeNotRegistered = errors.New("tipo de mensagem não registrado")

func lookupMsgType(name string) (reflect.Type, bool) {
	registryMu.RLock()
	t, ok := msgTypes[name]
	registryMu.RUnlock()
	return t, ok
}

// init registra mensagens simples do Bubble Tea que não têm tratamento
// especial. Chaves e redimensionamentos usam seus próprios tipos de evento.
func init() {
	RegisterMsg(tea.QuitMsg{})
	RegisterMsg(tea.InterruptMsg{})
	RegisterMsg(tea.SuspendMsg{})
	RegisterMsg(tea.ResumeMsg{})
	RegisterMsg(tea.FocusMsg{})
	RegisterMsg(tea.BlurMsg{})
	RegisterMsg(tea.PasteMsg{})
	RegisterMsg(tea.PasteStartMsg{})
	RegisterMsg(tea.PasteEndMsg{})
	RegisterMsg(tea.MouseClickMsg{})
	RegisterMsg(tea.MouseWheelMsg{})
	RegisterMsg(tea.MouseMotionMsg{})
}

type resizePayload struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type keyPayload struct {
	Text        string `json:"text,omitempty"`
	Mod         int    `json:"mod,omitempty"`
	Code        rune   `json:"code,omitempty"`
	ShiftedCode rune   `json:"shifted_code,omitempty"`
	BaseCode    rune   `json:"base_code,omitempty"`
	Repeat      bool   `json:"repeat,omitempty"`
	Release     bool   `json:"release,omitempty"`
}

type msgPayload struct {
	Type string          `json:"__type"`
	Data json.RawMessage `json:"__data"`
}

type unserializablePayload struct {
	Type string `json:"__unserializable"`
}

type cmdPayload struct {
	Name string `json:"name,omitempty"`
}

// envelopeProbe é usado para inspecionar o payload de um evento "msg" sem
// comprometer-se com um tipo Go específico.
type envelopeProbe struct {
	Unserializable json.RawMessage `json:"__unserializable"`
	Type           string          `json:"__type"`
	Data           json.RawMessage `json:"__data"`
}

// encodeMsg converte uma mensagem em (tipo de evento, payload JSON). O
// booleano lost indica que a mensagem não pôde ser serializada.
func encodeMsg(msg tea.Msg) (typ, data string, lost bool) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return "resize", marshalPayload(resizePayload{Width: m.Width, Height: m.Height}), false
	case tea.KeyPressMsg:
		return "key", marshalPayload(keyFromKey(tea.Key(m), false)), false
	case tea.KeyReleaseMsg:
		return "key", marshalPayload(keyFromKey(tea.Key(m), true)), false
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		name := "<nil>"
		if msg != nil {
			name = reflect.TypeOf(msg).String()
		}
		return "msg", marshalPayload(unserializablePayload{Type: name}), true
	}

	name := "<nil>"
	if msg != nil {
		name = reflect.TypeOf(msg).String()
		// Registrar o tipo aqui faz o replay em memória funcionar sem uma
		// chamada extra a RegisterMsg. Ao carregar um cassete em outro
		// processo, o registro explícito continua necessário.
		RegisterMsg(msg)
	}
	return "msg", marshalPayload(msgPayload{Type: name, Data: raw}), false
}

func keyFromKey(k tea.Key, release bool) keyPayload {
	return keyPayload{
		Text:        k.Text,
		Mod:         int(k.Mod),
		Code:        k.Code,
		ShiftedCode: k.ShiftedCode,
		BaseCode:    k.BaseCode,
		Repeat:      k.IsRepeat,
		Release:     release,
	}
}

func keyFromPayload(p keyPayload) tea.Key {
	return tea.Key{
		Text:        p.Text,
		Mod:         tea.KeyMod(p.Mod),
		Code:        p.Code,
		ShiftedCode: p.ShiftedCode,
		BaseCode:    p.BaseCode,
		IsRepeat:    p.Repeat,
	}
}

// marshalPayload serializa um payload interno. Os tipos usados aqui contêm
// apenas campos primitivos, então uma falha é impossível na prática; ainda
// assim não há panic.
func marshalPayload(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// decodeEvent transforma um evento em mensagem. Um aviso não vazio indica que
// o evento deve ser pulado, mas não invalida o cassete. Um erro indica dado
// corrompido ou tipo de evento desconhecido.
func decodeEvent(ev Event) (msg tea.Msg, warning string, err error) {
	switch ev.Type {
	case "resize":
		var p resizePayload
		if err := json.Unmarshal([]byte(ev.Data), &p); err != nil {
			return nil, "", fmt.Errorf("dados de resize inválidos: %w", err)
		}
		if p.Width < 0 {
			p.Width = 0
		}
		if p.Height < 0 {
			p.Height = 0
		}
		return tea.WindowSizeMsg{Width: p.Width, Height: p.Height}, "", nil

	case "key":
		var p keyPayload
		if err := json.Unmarshal([]byte(ev.Data), &p); err != nil {
			return nil, "", fmt.Errorf("dados de tecla inválidos: %w", err)
		}
		k := keyFromPayload(p)
		if p.Release {
			return tea.KeyReleaseMsg(k), "", nil
		}
		return tea.KeyPressMsg(k), "", nil

	case "msg":
		return decodeMsg(ev.Data)

	case "cmd":
		var p cmdPayload
		if ev.Data != "" && ev.Data != "null" {
			if err := json.Unmarshal([]byte(ev.Data), &p); err != nil {
				return nil, "", fmt.Errorf("dados de comando inválidos: %w", err)
			}
		}
		name := p.Name
		if name == "" {
			name = "?"
		}
		return nil, fmt.Sprintf("evento cmd %q não reproduzível; ignorado", name), nil

	default:
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownEvent, ev.Type)
	}
}

func decodeMsg(data string) (tea.Msg, string, error) {
	var probe envelopeProbe
	if err := json.Unmarshal([]byte(data), &probe); err != nil {
		return nil, "", fmt.Errorf("payload de mensagem inválido: %w", err)
	}

	if len(probe.Unserializable) > 0 {
		var name string
		if err := json.Unmarshal(probe.Unserializable, &name); err != nil || name == "" {
			name = "desconhecido"
		}
		return nil, fmt.Sprintf("mensagem não serializável (%s) ignorada", name), nil
	}

	if probe.Type == "" {
		return nil, "", errors.New("payload de mensagem sem o campo __type")
	}
	if probe.Type == "<nil>" {
		return nil, "", nil
	}

	msg, err := decodeRegistered(probe.Type, probe.Data)
	if err != nil {
		if errors.Is(err, errTypeNotRegistered) {
			return nil, fmt.Sprintf("tipo %q não registrado; evento ignorado (use record.RegisterMsg)", probe.Type), nil
		}
		return nil, "", err
	}
	return msg, "", nil
}

func decodeRegistered(name string, raw json.RawMessage) (tea.Msg, error) {
	t, ok := lookupMsgType(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errTypeNotRegistered, name)
	}

	var target reflect.Value
	pointer := t.Kind() == reflect.Pointer
	if pointer {
		target = reflect.New(t.Elem())
	} else {
		target = reflect.New(t)
	}
	if err := json.Unmarshal(raw, target.Interface()); err != nil {
		return nil, fmt.Errorf("decodificar mensagem %s: %w", name, err)
	}
	if pointer {
		return target.Interface(), nil
	}
	return target.Elem().Interface(), nil
}
