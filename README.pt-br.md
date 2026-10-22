# charm-record

Grave e reproduza sessões de TUI do Bubble Tea como cassetes JSON determinísticos para testes.

> [Read in English](README.md)

## Instalação

```bash
go get github.com/junhinhow/charm-record
```

## Uso

Um arquivo de teste completo que grava sob demanda e reproduz por padrão:

```go
package counter

import (
	"flag"
	"fmt"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/junhinhow/charm-record"
)

var update = flag.Bool("update", false, "regrava os cassetes")

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

func TestIncrementSession(t *testing.T) {
	const path = "testdata/incremento.cassette.json"

	if *update {
		rec := record.NewRecorder(Model{}, 80, 24)
		rec.AddMeta("cenario", "incremento")
		rec.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
		rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := rec.Save(path); err != nil {
			t.Fatal(err)
		}
	}

	c, err := record.Load(path)
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
	if got, want := rep.View().Content, "count=2"; got != want {
		t.Fatalf("view final = %q, quer %q", got, want)
	}
}
```

Grave uma vez e comite o cassete:

```bash
go test ./... -run TestIncrementSession -update
```

A CI roda apenas `go test ./...`: o replay não precisa de TTY, `time.Sleep` nem
timers reais. O mesmo cassete sempre produz os mesmos frames. O cassete é JSON
indentado e legível; `At` é o único campo temporal e guarda milissegundos
relativos ao início da gravação.

### Mensagens próprias

`WindowSizeMsg` vira um evento `resize` e mensagens de tecla viram eventos
`key`. Qualquer outra mensagem é gravada como `msg`, com o nome do tipo Go e o
payload JSON. Ao gravar, o tipo é registrado automaticamente no processo atual,
então gravar e reproduzir no mesmo teste não exige configuração extra. Para
reproduzir um cassete carregado em outro processo (por exemplo, na CI), registre
um protótipo uma vez:

```go
type loadedMsg struct{ Items []string }

func init() { record.RegisterMsg(loadedMsg{}) }
```

O registro usa `reflect.Type.String()` como chave (por exemplo
`counter.loadedMsg`). Registre o mesmo nome de pacote/tipo no binário de replay.

## API

| Símbolo | Descrição |
| --- | --- |
| `Version` | Versão atual do formato de cassete (`1`). |
| `Event` | Um passo da sessão: `Type`, `Data` (string JSON), `At` (ms relativos). |
| `Cassette` | Sessão gravada: versão, tamanho inicial, metadados e eventos. |
| `NewRecorder` | Envolve um modelo e inicia a gravação. |
| `Recorder.Update` | Grava uma mensagem e a repassa ao modelo. |
| `Recorder.View` | View atual do modelo envolvido. |
| `Recorder.Cassette` | Cópia instantânea do cassete. |
| `Recorder.AddMeta` | Adiciona um par chave/valor de metadados. |
| `Recorder.Save` | Grava JSON indentado determinístico. |
| `Recorder.Reset` | Limpa eventos, metadados e contadores. |
| `Load`, `LoadBytes` | Lê e valida um cassete. |
| `RegisterMsg` | Registra um protótipo de mensagem própria para o replay. |
| `NewReplayer` | Valida um cassete e prepara a reprodução. |
| `Replayer.Run` | Aplica todos os eventos restantes. |
| `Replayer.Step` | Aplica exatamente um evento; retorna `io.EOF` no fim. |
| `Replayer.View` / `Model` | View / modelo atual. |
| `Replayer.Frames` | Conteúdo da View após cada evento processado. |
| `Replayer.Warnings` | Eventos ignorados, com os motivos. |
| `MetaUnserializable` | Chave de metadados com o número de mensagens não serializáveis. |
| `ErrVersionMismatch`, `VersionError` | Incompatibilidade de versão do formato. |
| `ErrNilModel`, `ErrNilCassette`, `ErrUnknownEvent` | Erros tipados para casos de borda. |

## Notas de projeto

- **Saída determinística.** O JSON é indentado com dois espaços e as chaves de
  mapas são ordenadas. Salvar o mesmo cassete duas vezes produz bytes idênticos.
  Apenas `At` é temporal, e ele é relativo, nunca um timestamp absoluto.
- **Formato versionado.** Qualquer versão diferente de `Version` falha com um
  `*VersionError` que informa a versão encontrada e a esperada;
  `errors.Is(err, ErrVersionMismatch)` funciona.
- **Mensagens não serializáveis.** Se `json.Marshal` falhar, o evento vira
  `{"__unserializable": "<tipo>"}` e é contado em `Meta` sob
  `MetaUnserializable`. O replay pula o evento, acumula um aviso e continua. Um
  tipo não registrado no binário de replay recebe o mesmo tratamento.
- **Comandos não são gravados.** Um `tea.Cmd` é uma função, não um dado; por
  isso `Update` o repassa ao chamador, mas não o executa nem o armazena. O tipo
  de evento `cmd` é reconhecido para ferramentas externas e ignorado com aviso
  durante o replay.
- **Sem espera.** `At` é informativo. O replay é síncrono e o ignora.
- **Frames.** Há um frame por evento processado, inclusive eventos ignorados;
  `Frames` devolve `tea.View.Content` como string para comparações golden.
- **Uso único.** `Init` não é chamado, comandos retornados são ignorados e um
  `Replayer` deve ser executado uma vez. Comece com um modelo novo. `Recorder` e
  `Replayer` não são seguros para uso concorrente; o registro de mensagens é.
- **O tamanho inicial é metadado.** `Width` e `Height` descrevem a sessão, mas o
  replay não sintetiza um `WindowSizeMsg`. Grave um se o modelo precisar dele;
  programas Bubble Tea reais recebem um na inicialização.
- **Cassetes vazios são válidos.** Um cassete sem eventos carrega e reproduz
  como uma operação sem efeito.

## Licença

MIT
