package run

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/platform/form"
)

type questionRecord struct {
	question  interaction.Question
	commandID string
	response  *interaction.QuestionResponse
	closed    bool
}

func (c *conversationState) applyQuestion(event interaction.Event) {
	if c.questions == nil {
		c.questions = make(map[string]questionRecord)
	}
	record := c.questions[event.RequestID]
	switch event.Type {
	case interaction.EventQuestionRequested:
		record = questionRecord{question: *event.Question, commandID: event.CommandID}
		c.questionSequence = max(c.questionSequence, event.Sequence)
	case interaction.EventQuestionAnswered:
		record.response = event.QuestionResponse
	case interaction.EventQuestionCancelled, interaction.EventQuestionUncertain:
		record.closed = true
	default:
		return
	}
	c.questions[event.RequestID] = record
}

func (c *conversationState) validateQuestion(event interaction.Event) error {
	record, exists := c.questions[event.RequestID]
	switch event.Type {
	case interaction.EventQuestionRequested:
		if exists || event.Question == nil || event.CommandID == "" || event.CommandID != c.activeCommandID || event.RequestID == "" {
			return ErrJournal
		}
		if _, err := form.Compile(event.Question.Schema); err != nil {
			return ErrJournal
		}
	case interaction.EventQuestionAnswered:
		if !exists || record.closed || record.response != nil || event.QuestionResponse == nil || record.commandID != event.CommandID {
			return ErrJournal
		}
		if event.QuestionResponse.RequestID != event.RequestID || event.QuestionResponse.CommandID != event.CommandID || validateQuestionAnswer(record.question, event.QuestionResponse.Answer) != nil {
			return ErrJournal
		}
	case interaction.EventQuestionCancelled, interaction.EventQuestionUncertain:
		if !exists || record.commandID != event.CommandID {
			return ErrJournal
		}
	}
	return nil
}

func (c *conversation) commandHasQuestions(commandID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, question := range c.state.questions {
		if question.commandID == commandID {
			return true
		}
	}
	return false
}
