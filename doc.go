// Package record grava e reproduz sessões de TUI do Bubble Tea como cassetes
// determinísticos, pensados para testes.
//
// Um [Recorder] envolve um [tea.Model] e registra cada mensagem entregue a
// Update, além do tamanho inicial da tela. O resultado é um [Cassette], que
// pode ser salvo em JSON e reexecutado por um [Replayer] sem terminal, sem
// timers e sem [time.Sleep].
//
// Uso típico em um teste:
//
//	rec := record.NewRecorder(model, 80, 24)
//	rec.AddMeta("cenario", "lista-vazia")
//	rec.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
//	rec.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
//	if err := rec.Save("testdata/lista-vazia.json"); err != nil {
//		log.Fatal(err)
//	}
//
// E, na reprodução:
//
//	c, err := record.Load("testdata/lista-vazia.json")
//	if err != nil {
//		log.Fatal(err)
//	}
//	rep, err := record.NewReplayer(model, c)
//	if err != nil {
//		log.Fatal(err)
//	}
//	if err := rep.Run(); err != nil {
//		log.Fatal(err)
//	}
//	_ = rep.Frames() // uma View por evento, na ordem
//
// # Mensagens
//
// WindowSizeMsg vira um evento "resize" e mensagens de tecla viram eventos
// "key". As demais mensagens serializáveis viram eventos "msg" com o nome do
// tipo Go e o payload JSON. O Recorder registra o tipo automaticamente no
// processo atual, então gravar e reproduzir na mesma execução funciona sem
// configuração. Para reproduzir um cassete carregado em outro processo, o tipo
// precisa ser registrado com [RegisterMsg]:
//
//	func init() { record.RegisterMsg(minhaMsg{}) }
//
// Mensagens que não podem ser serializadas viram um evento "msg" com
// {"__unserializable": "<tipo>"}; o replay pula esses eventos e acumula um
// aviso em [Replayer.Warnings]. A contagem também fica em
// [Cassette.Meta] sob a chave [MetaUnserializable].
//
// # Determinismo
//
// O JSON é indentado e o mapa Meta é serializado com chaves ordenadas. O campo
// [Event.At] é o único dado temporal: milissegundos relativos ao início da
// gravação. Ele é informativo e nunca é usado para esperar durante o replay.
// Assim, duas execuções do mesmo cassete produzem exatamente as mesmas Views.
//
// # Limitações
//
// Comandos ([tea.Cmd]) não são gravados: eles são funções, não dados. O
// Recorder apenas os repassa ao chamador. Eventos "cmd" existem no formato
// para metadados de ferramentas externas, mas o replay os ignora com um aviso.
// O Replayer também não chama [tea.Model.Init]; o modelo deve chegar pronto
// para receber as mensagens. As dimensões iniciais do cassete são metadados:
// para atualizar o modelo, grave um WindowSizeMsg (o Bubble Tea envia um na
// inicialização de um programa real).
package record
