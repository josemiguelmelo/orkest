import { ChevronDown, FileInput, FileOutput, Plus, Trash2 } from 'lucide-react';
import type {
  ArtifactInput,
  ArtifactOutput,
  WorkflowStage,
} from '../../shared/api/generated/types.gen';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { WorkflowChoiceSelect } from './workflow-choice-select';
import '../settings/workflow-stage-integrations.css';
import './stage-artifacts.css';

export function StageArtifacts({
  stage,
  stages,
  disabled,
  onChange,
}: {
  stage: WorkflowStage;
  stages: WorkflowStage[];
  disabled: boolean;
  onChange: (values: Partial<WorkflowStage>) => void;
}) {
  const outputs = stage.artifactOutputs ?? [];
  const inputs = stage.artifactInputs ?? [];
  const total = outputs.length + inputs.length;
  const availableOutputs = stages
    .filter((candidate) => candidate.id !== stage.id)
    .flatMap((candidate) =>
      (candidate.artifactOutputs ?? []).map((output) => ({
        stageID: candidate.id,
        stageName: candidate.name,
        output,
      })),
    );
  function updateOutput(index: number, values: Partial<ArtifactOutput>) {
    onChange({
      artifactOutputs: outputs.map((output, current) =>
        current === index ? { ...output, ...values } : output,
      ),
    });
  }
  function updateInput(index: number, values: Partial<ArtifactInput>) {
    onChange({
      artifactInputs: inputs.map((input, current) =>
        current === index ? { ...input, ...values } : input,
      ),
    });
  }
  return (
    <details className="workflow-artifacts">
      <summary>
        <FileOutput size={16} aria-hidden="true" />
        <span>Artifacts</span>
        <span className="workflow-artifact-count">{total}</span>
        <ChevronDown className="workflow-artifact-chevron" size={16} aria-hidden="true" />
      </summary>
      <div className="workflow-artifacts-intro">
        <p>Connect the results produced in {stage.name} with the work that follows.</p>
        <small>
          Save the workflow to apply changes. Agents use these names to publish and read artifacts.
        </small>
      </div>
      <fieldset className="workflow-artifact-section">
        <legend>
          <span>1</span> Publish results
        </legend>
        <div className="workflow-artifact-section-heading">
          <div>
            <p>Results published when work in this stage completes.</p>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={disabled || outputs.length >= 50}
            onClick={() =>
              onChange({ artifactOutputs: [...outputs, { name: 'output', kind: 'plan' }] })
            }
          >
            <Plus size={14} /> Add output
          </Button>
        </div>
        {outputs.length === 0 ? (
          <div className="workflow-artifact-empty">
            <FileOutput size={24} aria-hidden="true" />
            <strong>No outputs configured yet</strong>
            <p>Add a result for agents to publish, such as a plan or pull request.</p>
          </div>
        ) : (
          outputs.map((output, index) => (
            <div className="workflow-artifact-card" key={index}>
              <div className="workflow-artifact-card-heading">
                <div>
                  <strong>
                    Output {index + 1} · {output.name || 'Choose a name'}
                  </strong>
                  <small>{output.kind || 'Choose a kind'}</small>
                </div>
              </div>
              <div className="workflow-artifact-fields">
                <Field label="Output name" hint="A short name agents can refer to.">
                  <input
                    disabled={disabled}
                    value={output.name}
                    onChange={(event) => updateOutput(index, { name: event.target.value })}
                  />
                </Field>
                <Field label="Output kind" hint="For example, plan or pull_request.">
                  <input
                    disabled={disabled}
                    value={output.kind}
                    onChange={(event) => updateOutput(index, { kind: event.target.value })}
                  />
                </Field>
              </div>
              <div className="workflow-artifact-card-footer">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  aria-label={`Remove output ${index + 1}`}
                  title="Remove output"
                  onClick={() =>
                    onChange({ artifactOutputs: outputs.filter((_, current) => current !== index) })
                  }
                >
                  <Trash2 size={14} aria-hidden="true" /> Remove output
                </Button>
              </div>
            </div>
          ))
        )}
      </fieldset>
      <fieldset className="workflow-artifact-section">
        <legend>
          <span>2</span> Use results from other stages
        </legend>
        <div className="workflow-artifact-section-heading">
          <div>
            <p>Bring an output from another stage into this stage.</p>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={disabled || inputs.length >= 50}
            onClick={() =>
              onChange({
                artifactInputs: [
                  ...inputs,
                  { name: 'input', sourceStageId: '', outputName: '', required: true },
                ],
              })
            }
          >
            <Plus size={14} /> Add input
          </Button>
        </div>
        {inputs.length === 0 ? (
          <div className="workflow-artifact-empty">
            <FileInput size={24} aria-hidden="true" />
            <strong>No inputs configured yet</strong>
            <p>Choose a result from another stage to make it available here.</p>
          </div>
        ) : (
          inputs.map((input, index) => (
            <div className="workflow-artifact-card" key={index}>
              <div className="workflow-artifact-card-heading">
                <div>
                  <strong>
                    Input {index + 1} · {input.name || 'Choose a name'}
                  </strong>
                  <small>{input.required ? 'Required before starting' : 'Optional input'}</small>
                </div>
              </div>
              <div className="workflow-artifact-fields">
                <Field label="Input name" hint="The local name used by this stage.">
                  <input
                    disabled={disabled}
                    value={input.name}
                    onChange={(event) => updateInput(index, { name: event.target.value })}
                  />
                </Field>
                <Field label="Source output">
                  <WorkflowChoiceSelect
                    disabled={disabled || availableOutputs.length === 0}
                    value={
                      input.sourceStageId
                        ? JSON.stringify([input.sourceStageId, input.outputName])
                        : ''
                    }
                    placeholder={
                      availableOutputs.length === 0
                        ? 'Add an output in another stage first'
                        : 'Choose a source output'
                    }
                    aria-required
                    groups={[
                      ...(!availableOutputs.some(
                        ({ stageID, output }) =>
                          stageID === input.sourceStageId && output.name === input.outputName,
                      ) && input.sourceStageId
                        ? [
                            {
                              label: 'Unavailable source',
                              options: [
                                {
                                  value: JSON.stringify([input.sourceStageId, input.outputName]),
                                  label: `Unavailable output (${input.outputName})`,
                                  description: 'Choose an existing output before saving.',
                                },
                              ],
                            },
                          ]
                        : []),
                      ...stages
                        .filter(
                          (candidate) =>
                            candidate.id !== stage.id && candidate.artifactOutputs?.length,
                        )
                        .map((candidate) => ({
                          label: candidate.name,
                          options: (candidate.artifactOutputs ?? []).map((output) => ({
                            value: JSON.stringify([candidate.id, output.name]),
                            label: `${candidate.name} / ${output.name}`,
                            description: `Kind: ${output.kind}`,
                          })),
                        })),
                    ]}
                    onChange={(value) => {
                      const [sourceStageId, outputName] = JSON.parse(value) as [string, string];
                      updateInput(index, { sourceStageId, outputName });
                    }}
                  />
                </Field>
              </div>
              <div className="workflow-artifact-preview">
                <FileInput size={14} aria-hidden="true" />
                <p>
                  Read <strong>{input.outputName || 'a source output'}</strong> as{' '}
                  <strong>{input.name || 'a local input'}</strong> in {stage.name}.
                </p>
              </div>
              <label className="workflow-artifact-required">
                <input
                  type="checkbox"
                  disabled={disabled}
                  checked={input.required}
                  onChange={(event) => updateInput(index, { required: event.target.checked })}
                />
                <span>
                  <strong>Required before starting</strong>
                  <small>Prevent this stage from starting until the input exists.</small>
                </span>
              </label>
              <div className="workflow-artifact-card-footer">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  aria-label={`Remove input ${index + 1}`}
                  title="Remove input"
                  onClick={() =>
                    onChange({ artifactInputs: inputs.filter((_, current) => current !== index) })
                  }
                >
                  <Trash2 size={14} aria-hidden="true" /> Remove input
                </Button>
              </div>
            </div>
          ))
        )}
      </fieldset>
    </details>
  );
}
