package codex_test

import (
	"context"
	stdjson "encoding/json"
	"fmt"

	"github.com/zhubiaook/codex/sdk/go"
)

func Example() {
	client, err := codex.NewClient(codex.ClientOptions{})
	if err != nil {
		return
	}
	thread := client.StartThread(codex.ThreadOptions{})
	turn, err := thread.Run(context.Background(), "Summarize this repository.", codex.TurnOptions{})
	if err != nil {
		return
	}
	fmt.Println(turn.FinalResponse)
}

func ExampleThread_RunStreamed() {
	client, err := codex.NewClient(codex.ClientOptions{})
	if err != nil {
		return
	}
	thread := client.StartThread(codex.ThreadOptions{})
	for event, err := range thread.RunStreamed(
		context.Background(), "Run the tests.", codex.TurnOptions{},
	) {
		if err != nil {
			return
		}
		switch event := event.(type) {
		case *codex.ItemCompletedEvent:
			fmt.Printf("completed %T\n", event.Item)
		case *codex.UnknownEvent:
			fmt.Printf("unknown event %q\n", event.UnknownType)
		}
	}
}

func ExampleThread_RunJSON() {
	type Summary struct {
		Text string `json:"text"`
	}
	schema := stdjson.RawMessage(`{
  "type": "object",
  "properties": {"text": {"type": "string"}},
  "required": ["text"],
  "additionalProperties": false
}`)
	client, err := codex.NewClient(codex.ClientOptions{})
	if err != nil {
		return
	}
	result, err := client.StartThread(codex.ThreadOptions{}).RunJSON[Summary](
		context.Background(),
		codex.StructuredInput{
			Text:        "Describe this screenshot.",
			LocalImages: []string{"./screenshot.png"},
		},
		schema,
		codex.TurnOptions{},
	)
	if err != nil {
		return
	}
	fmt.Println(result.Output.Text)
}
