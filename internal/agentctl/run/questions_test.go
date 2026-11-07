package run

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
)

var testQuestion = interaction.Question{Message: "Package manager?", Schema: json.RawMessage(`{"type":"object","properties":{"manager":{"type":"string","enum":["npm","pnpm"]}},"required":["manager"]}`)}

func questionConversation(t *testing.T) *conversation {
	t.Helper()
	c := newConversation(context.Background(), t.TempDir(), nil)
	session := &fakeRuntimeSession{done: make(chan struct{})}
	c.bind(runConfig(), session)
	c.journal = &Journal{file: successfulFile{}}
	c.state.activeCommandID = "command"
	c.state.commands = map[string]commandRecord{"command": {state: interaction.CommandRunning}}
	c.executionState = runmodel.StateRunning
	t.Cleanup(func() { _ = c.close(); _ = session.Close() })
	return c
}

func questionResponse(id string) interaction.QuestionResponse {
	return interaction.QuestionResponse{TaskID: "task", RunID: "run", SessionID: "session", CommandID: "command", RequestID: id, SubmissionID: "submission", Answer: interaction.QuestionAnswer{Action: "accept", Content: json.RawMessage(`{"manager":"pnpm"}`)}}
}

func TestQuestionAnswersAreDurableBoundAndIdempotent(t *testing.T) {
	c := questionConversation(t)
	id, pending, err := c.registerQuestion(context.Background(), "command", testQuestion)
	if err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().State != runmodel.StateWaitingInput {
		t.Fatal("question did not retain a waiting execution")
	}
	response := questionResponse(id)
	for _, mutate := range []func(*interaction.QuestionResponse){
		func(r *interaction.QuestionResponse) { r.TaskID = "other" },
		func(r *interaction.QuestionResponse) { r.RunID = "other" },
		func(r *interaction.QuestionResponse) { r.SessionID = "other" },
		func(r *interaction.QuestionResponse) { r.TaskSessionID = "other" },
		func(r *interaction.QuestionResponse) { r.CommandID = "other" },
		func(r *interaction.QuestionResponse) { r.Answer.Content = json.RawMessage(`{"manager":"invalid"}`) },
	} {
		invalid := response
		mutate(&invalid)
		if c.RespondQuestion(invalid) == nil {
			t.Fatal("invalid answer accepted")
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := c.RespondQuestion(response); err != nil {
				t.Errorf("identical submission: %v", err)
			}
		})
	}
	wg.Wait()
	answer := <-pending.response
	if string(answer.Content) != `{"manager":"pnpm"}` {
		t.Fatal("answer lost")
	}
	history, err := c.journal.Replay(0)
	if err != nil || len(history) != 2 || history[1].Type != interaction.EventQuestionAnswered {
		t.Fatalf("answer was not recorded once: %+v %v", history, err)
	}
	response.SubmissionID = "conflict"
	if !errors.Is(c.RespondQuestion(response), interaction.ErrQuestion) {
		t.Fatal("conflicting submission accepted")
	}
	c.removePendingQuestion(id, true)
	if c.Snapshot().State != runmodel.StateRunning {
		t.Fatal("question completion did not restore execution")
	}
	if !c.commandHasQuestions("command") {
		t.Fatal("question evidence lost for recovery")
	}
}

func TestQuestionPersistenceFailureNeverDeliversAnAnswer(t *testing.T) {
	c := questionConversation(t)
	id, pending, err := c.registerQuestion(context.Background(), "command", testQuestion)
	if err != nil {
		t.Fatal(err)
	}
	c.journal.file = failingFile{syncFailure: true}
	if !errors.Is(c.RespondQuestion(questionResponse(id)), ErrJournal) {
		t.Fatal("failed commit acknowledged")
	}
	select {
	case <-pending.response:
		t.Fatal("uncommitted answer delivered")
	default:
	}
	if c.Snapshot().State != runmodel.StateInterrupted {
		t.Fatal("journal failure did not fence execution")
	}
}

func TestQuestionCancellationAndMultipleWaits(t *testing.T) {
	c := questionConversation(t)
	ctx, cancel := context.WithCancel(context.Background())
	id, _, err := c.registerQuestion(ctx, "command", testQuestion)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := c.registerQuestion(context.Background(), "command", testQuestion)
	if err != nil || second == id {
		t.Fatal("question identities collided", err)
	}
	cancel()
	c.removePendingQuestion(id, false)
	if c.Snapshot().State != runmodel.StateWaitingInput {
		t.Fatal("closing one question hid another wait")
	}
	if !errors.Is(c.RespondQuestion(questionResponse(id)), interaction.ErrQuestion) {
		t.Fatal("closed question accepted an answer")
	}
	c.removePendingQuestion(second, false)
	if c.Snapshot().State != runmodel.StateRunning {
		t.Fatal("waiting state remained after questions closed")
	}
}

func TestQuestionReplayPreservesAnswerAndMonotonicIdentity(t *testing.T) {
	c := questionConversation(t)
	id, _, err := c.registerQuestion(context.Background(), "command", testQuestion)
	if err != nil {
		t.Fatal(err)
	}
	response := questionResponse(id)
	if err := c.RespondQuestion(response); err != nil {
		t.Fatal(err)
	}
	history, err := c.journal.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	restored := conversationState{activeCommandID: "command"}
	for _, event := range history {
		if err := restored.validate(event); err != nil {
			t.Fatal(err)
		}
		restored.apply(event)
	}
	if !sameQuestionResponse(*restored.questions[id].response, response) || restored.questionSequence == 0 {
		t.Fatal("replay lost question state")
	}
}

func TestRestoreNeverReplaysAQuestionInvocationWithoutEvidence(t *testing.T) {
	dir := t.TempDir()
	recordRecoveryHistory(t, dir, promptAcceptedEvent("command", "original"),
		interaction.Event{Type: interaction.EventPromptStarted, CommandID: "command"},
		interaction.Event{Type: interaction.EventQuestionRequested, CommandID: "command", RequestID: "question-1", Question: &testQuestion})
	process := newRecoverySession()
	process.safe = true // Generic history coverage does not prove question delivery.
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, dir, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.RestoreSession(); err != nil {
		t.Fatal(err)
	}
	if !controller.Status().QueuePaused {
		t.Fatal("lost question invocation did not pause recovery")
	}
	select {
	case <-process.calls:
		t.Fatal("question invocation replayed without evidence")
	default:
	}
	response := questionResponse("question-1")
	response.TaskSessionID = "task-session"
	if !errors.Is(controller.RespondQuestion(response), interaction.ErrQuestion) {
		t.Fatal("answer delivered to a lost invocation")
	}
}

func TestRecordedAnswerWithLostWaitRemainsUncertainAndIdempotent(t *testing.T) {
	c := questionConversation(t)
	id, _, err := c.registerQuestion(context.Background(), "command", testQuestion)
	if err != nil {
		t.Fatal(err)
	}
	response := questionResponse(id)
	if err := c.RespondQuestion(response); err != nil {
		t.Fatal(err)
	}
	c.removePendingQuestion(id, false)
	history, err := c.journal.Replay(0)
	if err != nil || history[len(history)-1].Type != interaction.EventQuestionUncertain {
		t.Fatal("lost answer delivery was not visible", err)
	}
	if err := c.RespondQuestion(response); err != nil {
		t.Fatal("recorded answer retry was not acknowledged", err)
	}
}
