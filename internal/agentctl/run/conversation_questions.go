package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	"github.com/josemiguelmelo/orkest/internal/platform/form"
)

type pendingQuestion struct {
	ctx      context.Context
	response chan interaction.QuestionAnswer
}

func (c *conversation) waitQuestion(ctx context.Context, commandID string, question interaction.Question) (interaction.QuestionAnswer, error) {
	id, pending, err := c.registerQuestion(ctx, commandID, question)
	if err != nil {
		return interaction.QuestionAnswer{}, err
	}
	delivered := false
	defer func() { c.removePendingQuestion(id, delivered) }()
	select {
	case answer := <-pending.response:
		delivered = true
		return answer, nil
	case <-ctx.Done():
		return interaction.QuestionAnswer{}, ctx.Err()
	case <-c.ctx.Done():
		return interaction.QuestionAnswer{}, c.ctx.Err()
	}
}

func (c *conversation) registerQuestion(ctx context.Context, commandID string, question interaction.Question) (string, *pendingQuestion, error) {
	if len(question.Message) > form.MaxBytes || len(question.ToolCallID) > 200 {
		return "", nil, interaction.ErrQuestion
	}
	if _, err := form.Compile(question.Schema); err != nil {
		return "", nil, err
	}
	question.Schema = append(json.RawMessage(nil), question.Schema...)
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	valid := !c.closed && c.ctx.Err() == nil && ctx.Err() == nil && !c.state.cancelled && c.executionState != runmodel.StateInterrupted && c.state.activeCommandID == commandID && len(c.state.questions) < interaction.MaxQuestions
	id := fmt.Sprintf("question-%d", c.state.questionSequence+1)
	c.mu.RUnlock()
	if !valid {
		return "", nil, interaction.ErrQuestion
	}
	if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventQuestionRequested, CommandID: commandID, RequestID: id, Question: &question}); err != nil {
		return "", nil, err
	}
	pending := &pendingQuestion{ctx: ctx, response: make(chan interaction.QuestionAnswer, 1)}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.questions == nil {
		c.questions = make(map[string]*pendingQuestion)
	}
	c.questions[id] = pending
	c.refreshWaitingStateUnderLock()
	return id, pending, nil
}

func (c *conversation) removePendingQuestion(id string, delivered bool) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	record := c.state.questions[id]
	uncertain := !c.state.cancelled && (c.closed || c.ctx.Err() != nil || c.executionState == runmodel.StateInterrupted)
	c.mu.RUnlock()
	if !record.closed && !delivered {
		kind := interaction.EventQuestionCancelled
		if uncertain || record.response != nil {
			kind = interaction.EventQuestionUncertain
		}
		_ = c.commitEventUnderEventLock(interaction.Event{Type: kind, CommandID: record.commandID, RequestID: id})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.questions, id)
	c.refreshWaitingStateUnderLock()
}

// RespondQuestion records the answer before notifying the live invocation.
// An identical retry acknowledges that record even if the invocation has ended.
func (c *conversation) RespondQuestion(response interaction.QuestionResponse) error {
	response.Answer.Content = append(json.RawMessage(nil), response.Answer.Content...)
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	record, exists := c.state.questions[response.RequestID]
	valid := exists && c.config != nil && c.session != nil && response.TaskID == c.config.TaskID && response.RunID == c.config.RunID && response.TaskSessionID == c.config.TaskSessionID && response.SessionID == c.session.SessionID() && response.CommandID == record.commandID && response.SubmissionID != "" && len(response.SubmissionID) <= 200
	pending := c.questions[response.RequestID]
	available := valid && !record.closed && !c.closed && !c.state.cancelled && c.executionState != runmodel.StateInterrupted && c.state.activeCommandID == record.commandID && pending != nil && pending.ctx.Err() == nil && c.ctx.Err() == nil
	c.mu.RUnlock()
	if !valid {
		return interaction.ErrQuestion
	}
	if record.response != nil {
		if sameQuestionResponse(*record.response, response) {
			return nil
		}
		return interaction.ErrQuestion
	}
	if !available {
		return interaction.ErrQuestion
	}
	if err := validateQuestionAnswer(record.question, response.Answer); err != nil {
		return err
	}
	if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventQuestionAnswered, CommandID: response.CommandID, RequestID: response.RequestID, QuestionResponse: &response}); err != nil {
		return err
	}
	answer := response.Answer
	answer.Content = append(json.RawMessage(nil), answer.Content...)
	pending.response <- answer
	return nil
}

func validateQuestionAnswer(question interaction.Question, answer interaction.QuestionAnswer) error {
	switch answer.Action {
	case "accept":
		schema, err := form.Compile(question.Schema)
		if err != nil {
			return err
		}
		return form.ValidateAnswer(schema, answer.Content)
	case "decline", "cancel":
		if len(answer.Content) == 0 || string(answer.Content) == "null" {
			return nil
		}
	}
	return interaction.ErrQuestion
}

func sameQuestionResponse(left, right interaction.QuestionResponse) bool {
	canonical := func(value interaction.QuestionResponse) string {
		var content any
		decoder := json.NewDecoder(bytes.NewReader(value.Answer.Content))
		decoder.UseNumber()
		_ = decoder.Decode(&content)
		value.Answer.Content, _ = json.Marshal(content)
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	return canonical(left) == canonical(right)
}

// Caller holds mu. Interruptions and terminal states always take precedence.
func (c *conversation) refreshWaitingStateUnderLock() {
	if c.executionState != runmodel.StateRunning && c.executionState != runmodel.StateWaitingInput && c.executionState != runmodel.StateWaitingPermission {
		return
	}
	c.executionState = runmodel.StateRunning
	if len(c.questions) > 0 {
		c.executionState = runmodel.StateWaitingInput
	}
	if len(c.pending) > 0 {
		c.executionState = runmodel.StateWaitingPermission
	}
}
