import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import type {
  WorkflowAutomationRule,
  WorkspaceWorkflow,
} from '../../shared/api/generated/types.gen';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { ErrorState, Loading } from '../../shared/ui/common';
import {
  stageConnectionsQuery,
  stageProvidersQuery,
} from '../settings/workflow-stage-integrations';
import { IntegrationParameters, parameterDefaults } from './integration-parameters';
import type { JsonValue } from '../integrations/schema';

export function StageIntegrationRules({
  definition,
  stageID,
  disabled,
  onChange,
}: {
  definition: WorkspaceWorkflow;
  stageID: string;
  disabled: boolean;
  onChange: (rules: WorkflowAutomationRule[]) => void;
}) {
  const { scope } = useApp();
  const connections = useQuery(stageConnectionsQuery(scope));
  const providers = useQuery(stageProvidersQuery(scope.userID));
  const stage = definition.stages.find((item) => item.id === stageID);
  const artifactInputs = (stage?.artifactInputs ?? [])
    .filter((input) => {
      const source = definition.stages.find((item) => item.id === input.sourceStageId);
      const output = source?.artifactOutputs?.find((item) => item.name === input.outputName);
      return output?.kind === 'code_review' || output?.kind === 'pull_request';
    })
    .map((input) => input.name);
  const rules = definition.rules.filter(
    (rule) => rule.sourceStageId === stageID && rule.event === 'integration_action',
  );
  function patch(id: string, values: Partial<WorkflowAutomationRule>) {
    onChange(definition.rules.map((rule) => (rule.id === id ? { ...rule, ...values } : rule)));
  }
  return (
    <details className="workflow-stage-integrations">
      <summary>External integration callbacks ({rules.length})</summary>
      {connections.isPending || providers.isPending ? (
        <Loading />
      ) : connections.isError || providers.isError ? (
        <ErrorState error={connections.error ?? providers.error} />
      ) : (
        <>
          {rules.map((rule) => {
            const connection = connections.data.find(
              (item) => item.id === rule.integration?.connectionId,
            );
            const provider = providers.data.find((item) => item.provider === connection?.provider);
            const selectedAction = provider?.actions.find(
              (item) => item.id === rule.integration?.actionId,
            );
            return (
              <div key={rule.id} className="workflow-integration-rule">
                <Field label="Enable callback">
                  <input
                    type="checkbox"
                    disabled={disabled}
                    checked={rule.enabled}
                    onChange={(event) => patch(rule.id, { enabled: event.target.checked })}
                  />
                </Field>
                <Field label="Integration connection">
                  <select
                    required
                    disabled={disabled}
                    value={rule.integration?.connectionId ?? ''}
                    onChange={(event) => {
                      const next = connections.data.find((item) => item.id === event.target.value);
                      const action = providers.data.find((item) => item.provider === next?.provider)
                        ?.actions[0];
                      patch(rule.id, {
                        integration: {
                          connectionId: event.target.value,
                          actionId: action?.id ?? '',
                          parameters: parameterDefaults(action?.parametersSchema ?? {}),
                        },
                      });
                    }}
                  >
                    <option value="">Choose connection</option>
                    {!connection && rule.integration?.connectionId && (
                      <option value={rule.integration.connectionId}>
                        Unavailable connection ({rule.integration.connectionId})
                      </option>
                    )}
                    {connections.data.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                        {item.enabled ? '' : ' (disabled)'}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Integration action">
                  <select
                    required
                    disabled={disabled}
                    value={rule.integration?.actionId ?? ''}
                    onChange={(event) => {
                      const action = provider?.actions.find(
                        (item) => item.id === event.target.value,
                      );
                      patch(rule.id, {
                        integration: {
                          connectionId: rule.integration?.connectionId ?? '',
                          actionId: event.target.value,
                          parameters: parameterDefaults(action?.parametersSchema ?? {}),
                        },
                      });
                    }}
                  >
                    <option value="">Choose action</option>
                    {!selectedAction && rule.integration?.actionId && (
                      <option value={rule.integration.actionId}>
                        Unavailable action ({rule.integration.actionId})
                      </option>
                    )}
                    {provider?.actions.map((action) => (
                      <option key={action.id} value={action.id}>
                        {action.name}
                      </option>
                    ))}
                  </select>
                </Field>
                {selectedAction && rule.integration && (
                  <IntegrationParameters
                    schema={selectedAction.parametersSchema}
                    value={rule.integration.parameters as Record<string, JsonValue>}
                    artifactInputs={artifactInputs}
                    disabled={disabled}
                    onChange={(parameters) =>
                      patch(rule.id, { integration: { ...rule.integration!, parameters } })
                    }
                  />
                )}
                {connection && !connection.enabled && (
                  <p role="status" className="muted">
                    This connection is disabled. Its callbacks will not run until it is enabled.
                  </p>
                )}
                {!selectedAction && (
                  <p role="status" className="muted">
                    Choose an available connection and action. Existing settings are preserved until
                    you change them.
                  </p>
                )}
                <Field label="Workflow callback">
                  <select
                    disabled={disabled}
                    value={rule.action}
                    onChange={(event) => {
                      if (event.target.value === 'call_agent') {
                        const rest = { ...rule };
                        delete rest.targetStageId;
                        onChange(
                          definition.rules.map((item) =>
                            item.id === rule.id
                              ? {
                                  ...rest,
                                  action: 'call_agent',
                                  promptText: rule.promptText ?? '',
                                  sessionPolicy: 'continue_or_create_if_absent',
                                }
                              : item,
                          ),
                        );
                      } else {
                        const rest = { ...rule };
                        delete rest.promptText;
                        delete rest.sessionPolicy;
                        onChange(
                          definition.rules.map((item) =>
                            item.id === rule.id
                              ? { ...rest, action: 'move_task', targetStageId: '' }
                              : item,
                          ),
                        );
                      }
                    }}
                  >
                    <option value="move_task">Move task to a stage</option>
                    <option value="call_agent">Call assigned agent with a prompt</option>
                  </select>
                </Field>
                {rule.action === 'call_agent' ? (
                  <>
                    <Field
                      label="Agent callback prompt"
                      hint="External review context accompanies this prompt. The task stays in its current stage when the callback starts."
                    >
                      <textarea
                        disabled={disabled}
                        required
                        maxLength={32768}
                        value={rule.promptText ?? ''}
                        onChange={(event) => patch(rule.id, { promptText: event.target.value })}
                      />
                    </Field>
                    <Field label="Agent callback session">
                      <select
                        disabled={disabled}
                        value={rule.sessionPolicy ?? 'continue_or_create_if_absent'}
                        onChange={(event) =>
                          patch(rule.id, {
                            sessionPolicy: event.target.value as
                              'continue_or_create_if_absent' | 'new',
                          })
                        }
                      >
                        <option value="continue_or_create_if_absent">
                          Continue compatible conversation; create if absent
                        </option>
                        <option value="new">Start a new conversation</option>
                      </select>
                    </Field>
                  </>
                ) : (
                  <Field label="Callback target stage">
                    <select
                      required
                      disabled={disabled}
                      value={rule.targetStageId ?? ''}
                      onChange={(event) => patch(rule.id, { targetStageId: event.target.value })}
                    >
                      <option value="">Choose destination</option>
                      {definition.stages
                        .filter(
                          (item) => item.id !== stageID && item.id !== definition.closedStageId,
                        )
                        .map((item) => (
                          <option key={item.id} value={item.id}>
                            {item.name}
                          </option>
                        ))}
                    </select>
                  </Field>
                )}
                <Button
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => onChange(definition.rules.filter((item) => item.id !== rule.id))}
                >
                  Remove callback
                </Button>
              </div>
            );
          })}
          {stageID !== definition.closedStageId && (
            <Button
              variant="outline"
              disabled={disabled || connections.data.length === 0}
              onClick={() =>
                onChange([
                  ...definition.rules,
                  {
                    id: crypto.randomUUID(),
                    enabled: true,
                    event: 'integration_action',
                    sourceStageId: stageID,
                    action: 'move_task',
                    targetStageId: '',
                    integration: { connectionId: '', actionId: '', parameters: {} },
                  },
                ])
              }
            >
              Add integration callback
            </Button>
          )}
          {connections.data.length === 0 && (
            <p className="muted">
              Create a stage integration connection in workspace settings first.
            </p>
          )}
        </>
      )}
    </details>
  );
}
