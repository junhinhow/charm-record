package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Version é a versão do formato de cassete. Cassetes com outra versão são
// rejeitados com [ErrVersionMismatch].
const Version = 1

// MetaUnserializable é a chave de [Cassette.Meta] que recebe o número de
// mensagens que não puderam ser serializadas durante a gravação.
const MetaUnserializable = "record.unserializable"

var (
	// ErrVersionMismatch indica que o cassete foi gravado em uma versão do
	// formato diferente de [Version].
	ErrVersionMismatch = errors.New("record: versão de cassete incompatível")

	// ErrNilModel indica que uma operação recebeu um modelo nil.
	ErrNilModel = errors.New("record: modelo nil")

	// ErrNilCassette indica que uma operação recebeu um cassete nil.
	ErrNilCassette = errors.New("record: cassete nil")

	// ErrUnknownEvent indica que um evento tem um Type não reconhecido.
	ErrUnknownEvent = errors.New("record: tipo de evento desconhecido")
)

// VersionError descreve uma incompatibilidade de versão de cassete.
type VersionError struct {
	Found    int // versão encontrada no arquivo
	Expected int // versão esperada por esta biblioteca
}

// Error implementa error.
func (e *VersionError) Error() string {
	return fmt.Sprintf("record: versão de cassete %d incompatível (esperada %d)", e.Found, e.Expected)
}

// Unwrap permite usar errors.Is(err, [ErrVersionMismatch]).
func (e *VersionError) Unwrap() error { return ErrVersionMismatch }

// Event é um passo da sessão gravada.
type Event struct {
	Type string `json:"type"` // "msg", "key", "resize" ou "cmd"
	Data string `json:"data"` // payload serializado em JSON
	At   int64  `json:"at"`   // milissegundos relativos ao início da gravação
}

// Cassette é a sessão gravada.
type Cassette struct {
	Version int               `json:"version"`
	Width   int               `json:"width"`
	Height  int               `json:"height"`
	Meta    map[string]string `json:"meta"` // serializado com chaves ordenadas
	Events  []Event           `json:"events"`
}

// Load lê um cassete JSON de path e valida a versão do formato.
func Load(path string) (*Cassette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("record: ler cassete %q: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytes decodifica um cassete JSON e valida a versão do formato.
func LoadBytes(data []byte) (*Cassette, error) {
	var c Cassette
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("record: decodificar cassete: %w", err)
	}
	if c.Version != Version {
		return nil, &VersionError{Found: c.Version, Expected: Version}
	}
	c.normalize()
	return &c, nil
}

// normalize garante que Meta e Events sejam coleções vazias, e não nil, para
// que salvar e recarregar um cassete vazio produza sempre o mesmo JSON.
func (c *Cassette) normalize() {
	if c.Meta == nil {
		c.Meta = map[string]string{}
	}
	if c.Events == nil {
		c.Events = []Event{}
	}
}

// marshal serializa o cassete com indentação fixa e uma quebra de linha final.
// encoding/json ordena as chaves de mapas, o que torna a saída determinística.
func (c *Cassette) marshal() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("record: codificar cassete: %w", err)
	}
	return append(data, '\n'), nil
}
