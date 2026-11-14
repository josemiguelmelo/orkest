import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import type {
  WorkflowAutomationRule,
  WorkspaceWorkflow,
} from '../../shared/api/generated/types.gen';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { ErrorState, Loading } from '../../shared/ui/common';
import { stageConnectionsQuery, stageProvidersQuery } from '../../shared/api/queries';
import { IntegrationParameters, parameterDefaults } from './integration-parameters';
import type { JsonValue } from '../integrations/schema';
import { WorkflowChoiceSelect, type WorkflowChoice } from './workflow-choice-select';

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
  const stageName = stage?.name ?? stageID;
  const completionStage = definition.stages.find((item) => item.id === definition.closedStageId);
  const otherTargetStages = definition.stages.filter(
    (item) => item.id !== stageID && item.id !== definition.closedStageId,
  );
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
      <summary>
        External integrations <span className="workflow-integration-count">{rules.length}</span>
      </summary>
      {connections.isPending || providers.isPending ? (
        <Loading />
      ) : connections.isError || providers.isError ? (
        <ErrorState error={connections.error ?? providers.error} />
      ) : (
        <>
          <div className="workflow-integration-intro">
            <p>
              Connect a provider action to this stage. A matching event can move the task or prompt
              its assigned agent.
            </p>
            <small>
              Save the workflow to apply callback changes. Existing events are not replayed.
            </small>
          </div>
          {rules.map((rule) => {
            const connection = connections.data.find(
              (item) => item.id === rule.integration?.connectionId,
            );
            const provider = providers.data.find((item) => item.provider === connection?.provider);
            const selectedAction = provider?.actions.find(
              (item) => item.id === rule.integration?.actionId,
            );
            const connectionOptions: WorkflowChoice[] = [
              ...(!connection && rule.integration?.connectionId
                ? [
                    {
                      value: rule.integration.connectionId,
                      label: `Unavailable connection (${rule.integration.connectionId})`,
                      description: 'This saved connection is no longer available.',
                    },
                  ]
                : []),
              ...connections.data.map((item) => ({
                value: item.id,
                label: item.name,
                description: [
                  item.provider?.replaceAll('_', ' '),
                  item.repositoryIds?.length
                    ? `${item.repositoryIds.length} ${item.repositoryIds.length === 1 ? 'repository' : 'repositories'}`
                    : undefined,
                  item.enabled ? undefined : 'Paused',
                ]
                  .filter(Boolean)
                  .join(' · '),
              })),
            ];
            const actionOptions: WorkflowChoice[] = [
              ...(!selectedAction && rule.integration?.actionId
                ? [
                    {
                      value: rule.integration.actionId,
                      label: `Unavailable action (${rule.integration.actionId})`,
                      description: 'This saved action is not provided by the selected connection.',
                    },
                  ]
                : []),
              ...(provider?.actions.map((action) => ({
                value: action.id,
                label: action.name,
              })) ?? []),
            ];
            const targetGroups = [
              ...(stage
                ? [
                    {
                      label: 'Keep task in this stage',
                      options: [
                        {
                          value: stage.id,
                          label: stage.name,
                          description: 'Run the callback without changing the task stage.',
                        },
                      ],
                    },
                  ]
                : []),
              ...(otherTargetStages.length
                ? [
                    {
                      label: 'Move task to another stage',
                      options: otherTargetStages.map((item) => ({
                        value: item.id,
                        label: item.name,
                        ...(item.description ? { description: item.description } : {}),
                      })),
                    },
                  ]
                : []),
              ...(completionStage
                ? [
                    {
                      label: 'Complete task',
                      options: [
                        {
                          value: completionStage.id,
                          label: completionStage.name,
                          description: 'Marks the task as done.',
                        },
                      ],
                    },
                  ]
                : []),
            ];
            return (
              <div key={rule.id} className="workflow-integration-rule">
                <div className="workflow-integration-rule-heading">
                  <div>
                    <strong>{selectedAction?.name ?? 'Integration callback'}</strong>
                    <small>Runs while the task is in {stageName}.</small>
                  </div>
                  <Field label="Enabled">
                    <input
                      type="checkbox"
                      disabled={disabled}
                      checked={rule.enabled}
                      onChange={(event) => patch(rule.id, { enabled: event.target.checked })}
                    />
                  </Field>
                </div>
                <Field
                  label="Integration connection"
                  hint="Choose a workspace connection. Disabled connections are shown so saved rules remain editable."
                >
                  <WorkflowChoiceSelect
                    disabled={disabled}
                    value={rule.integration?.connectionId ?? ''}
                    placeholder="Choose a connection"
                    aria-required
                    options={connectionOptions}
                    onChange={(value) => {
                      const next = connections.data.find((item) => item.id === value);
                      const action = providers.data.find((item) => item.provider === next?.provider)
                        ?.actions[0];
                      patch(rule.id, {
                        integration: {
                          connectionId: value,
                          actionId: action?.id ?? '',
                          parameters: parameterDefaults(action?.parametersSchema ?? {}),
                        },
                      });
                    }}
                  />
                </Field>
                <Field label="Integration action">
                  <WorkflowChoiceSelect
                    disabled={disabled || !connection}
                    value={rule.integration?.actionId ?? ''}
                    placeholder="Choose an action"
                    aria-required
                    options={actionOptions}
                    onChange={(value) => {
                      const action = provider?.actions.find((item) => item.id === value);
                      patch(rule.id, {
                        integration: {
                          connectionId: rule.integration?.connectionId ?? '',
                          actionId: value,
                          parameters: parameterDefaults(action?.parametersSchema ?? {}),
                        },
                      });
                    }}
                  />
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
                {selectedAction && artifactInputs.length === 0 && (
                  <p className="workflow-integration-hint" role="status">
                    This stage has no pull request or code review artifact input yet. Add one to the
                    stage before configuring this action.
                  </p>
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
                  <WorkflowChoiceSelect
                    disabled={disabled}
                    value={rule.action}
                    placeholder="Choose a callback"
                    options={[
                      {
                        value: 'move_task',
                        label: 'Move task to a stage',
                        description: 'Change the task’s workflow stage.',
                      },
                      {
                        value: 'call_agent',
                        label: 'Call assigned agent with a prompt',
                        description: 'Send review context to the assigned agent.',
                      },
                    ]}
                    onChange={(value) => {
                      if (value === 'call_agent') {
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
                  />
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
                      <WorkflowChoiceSelect
                        disabled={disabled}
                        value={rule.sessionPolicy ?? 'continue_or_create_if_absent'}
                        placeholder="Choose a conversation"
                        options={[
                          {
                            value: 'continue_or_create_if_absent',
                            label: 'Continue this conversation',
                            description: 'Create one only if no compatible conversation exists.',
                          },
                          {
                            value: 'new',
                            label: 'Start a new conversation',
                            description: 'Keep this callback separate from earlier agent work.',
                          },
                        ]}
                        onChange={(value) =>
                          patch(rule.id, {
                            sessionPolicy: value as 'continue_or_create_if_absent' | 'new',
                          })
                        }
                      />
                    </Field>
                  </>
                ) : (
                  <Field
                    label="Callback target stage"
                    hint="Choose where the task should go when this provider event arrives. The current stage keeps it here; the closed stage marks the task done."
                  >
                    <WorkflowChoiceSelect
                      disabled={disabled}
                      value={rule.targetStageId ?? ''}
                      groups={targetGroups}
                      aria-required
                      onChange={(value) => patch(rule.id, { targetStageId: value })}
                      placeholder="Choose where the task should go"
                    />
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
            <div className="workflow-integration-no-connection">
              <strong>No stage connections are available.</strong>
              <span>Create one in workspace settings, then return here to choose its action.</span>
              <a
                href={`/organizations/${encodeURIComponent(scope.orgID)}/workspaces/${encodeURIComponent(scope.workspaceID)}/settings`}
              >
                Open workspace settings
              </a>
            </div>
          )}
        </>
      )}
    </details>
  );
}
