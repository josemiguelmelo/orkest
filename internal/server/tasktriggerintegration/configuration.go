package tasktriggerintegration

import (
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type ConfigurationPlan struct {
	Integration    Integration
	Create         bool
	Workflows      []workflowmodel.Definition
	ResetDiscovery bool
}

func PlanConfiguration(i Integration, create bool, workflows []workflowmodel.Definition) (ConfigurationPlan, error) {
	for _, rule := range i.Rules {
		found := false
		for _, definition := range workflows {
			if definition.ID != rule.WorkflowID {
				continue
			}
			if err := ValidateRuleWorkflow(rule, definition); err != nil {
				return ConfigurationPlan{}, err
			}
			found = true
			break
		}
		if !found {
			return ConfigurationPlan{}, problem.ErrInvalid
		}
	}
	return ConfigurationPlan{Integration: i, Create: create, Workflows: workflows, ResetDiscovery: true}, nil
}
