package record_test

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	record "github.com/junhinhow/charm-record"
)

type exampleModel struct {
	count int
}

func (m exampleModel) Init() tea.Cmd { return nil }

func (m exampleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "+":
			m.count++
		case "-":
			m.count--
		}
	}
	return m, nil
}

func (m exampleModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("count=%d", m.count))
}

func Example() {
	rec := record.NewRecorder(exampleModel{}, 80, 24)
	rec.AddMeta("cenario", "soma")
	rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	rec.Update(tea.KeyPressMsg{Code: '+', Text: "+"})

	rep, err := record.NewReplayer(exampleModel{}, rec.Cassette())
	if err != nil {
		panic(err)
	}
	if err := rep.Run(); err != nil {
		panic(err)
	}

	c := rec.Cassette()
	fmt.Println(c.Version, len(c.Events), c.Meta["cenario"])
	fmt.Println(rep.View().Content)

	// Output:
	// 1 2 soma
	// count=2
}
