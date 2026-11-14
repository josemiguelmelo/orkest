import './integration-parameters.css';
import { Field } from '../../shared/ui/form';
import type { JsonValue } from '../integrations/schema';
import { WorkflowChoiceSelect, WorkflowMultiChoiceSelect } from './workflow-choice-select';

type Schema = Record<string, unknown>;
function object(value: unknown): Schema {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Schema)
    : {};
}

function optionLabel(name: string, value: string) {
  if (name === 'mergePolicy') {
    if (value === 'all') return 'All linked PRs must be merged';
    if (value === 'any') return 'Any linked PR can be merged';
  }
  if (name === 'reviewStates') {
    if (value === 'changes_requested') return 'Changes requested';
    if (value === 'approved') return 'Approved';
    if (value === 'commented') return 'Commented';
  }
  return value.replaceAll('_', ' ').replace(/^\w/, (letter) => letter.toUpperCase());
}

export function parameterDefaults(schema: Schema): Record<string, JsonValue> {
  const result: Record<string, JsonValue> = {};
  for (const [name, raw] of Object.entries(object(schema.properties))) {
    const field = object(raw);
    if (field.default !== undefined) result[name] = structuredClone(field.default) as JsonValue;
  }
  return result;
}

// Provider catalog schemas determine the form. Unknown field types fail visibly.
export function IntegrationParameters({
  schema,
  value,
  onChange,
  disabled,
  artifactInputs = [],
}: {
  schema: Schema;
  value: Record<string, JsonValue>;
  onChange: (value: Record<string, JsonValue>) => void;
  disabled: boolean;
  artifactInputs?: string[];
}) {
  const required = Array.isArray(schema.required) ? schema.required : [];
  return (
    <>
      {Object.entries(object(schema.properties)).map(([name, raw]) => {
        const field = object(raw);
        const label = typeof field.title === 'string' ? field.title : name;
        const hint =
          typeof field.description === 'string'
            ? field.description
            : name === 'requireCurrentHead'
              ? 'Ignore reviews on older commits of the pull request.'
              : undefined;
        const change = (next: JsonValue) => onChange({ ...value, [name]: next });
        const choices = Array.isArray(field.enum)
          ? field.enum.filter((entry): entry is string => typeof entry === 'string')
          : [];
        const isRequired = required.includes(name);
        if (name === 'artifactInputName')
          return (
            <Field key={name} label={label} {...(hint ? { hint } : {})}>
              <WorkflowChoiceSelect
                disabled={disabled}
                value={typeof value[name] === 'string' ? value[name] : ''}
                placeholder="Choose a PR artifact input"
                aria-required
                options={[
                  ...(typeof value[name] === 'string' &&
                  value[name] !== '' &&
                  !artifactInputs.includes(value[name])
                    ? [
                        {
                          value: value[name],
                          label: `Unavailable artifact input (${value[name]})`,
                          description: 'This saved input is no longer available in the stage.',
                        },
                      ]
                    : []),
                  ...artifactInputs.map((input) => ({ value: input, label: input })),
                ]}
                onChange={(next) => change(next)}
              />
            </Field>
          );
        if (field.type === 'boolean')
          return (
            <Field key={name} label={label} {...(hint ? { hint } : {})}>
              <input
                type="checkbox"
                role="switch"
                className="workflow-parameter-toggle"
                disabled={disabled}
                checked={value[name] === true}
                onChange={(event) => change(event.target.checked)}
              />
            </Field>
          );
        if (field.type === 'array' && object(field.items).type === 'string') {
          const options = object(field.items).enum;
          const selected = Array.isArray(value[name]) ? (value[name] as string[]) : [];
          if (Array.isArray(options))
            return (
              <Field key={name} label={label} {...(hint ? { hint } : {})}>
                <WorkflowMultiChoiceSelect
                  disabled={disabled}
                  value={selected}
                  placeholder="Choose review states"
                  aria-required={isRequired}
                  options={options
                    .filter((entry): entry is string => typeof entry === 'string')
                    .map((entry) => ({ value: entry, label: optionLabel(name, entry) }))}
                  onChange={(next) => change(next)}
                />
              </Field>
            );
          return (
            <Field key={name} label={label} hint={hint ?? 'Separate values with commas.'}>
              <input
                disabled={disabled}
                value={selected.join(', ')}
                onChange={(event) =>
                  change(
                    event.target.value
                      .split(',')
                      .map((item) => item.trim())
                      .filter(Boolean),
                  )
                }
              />
            </Field>
          );
        }
        if (field.type === 'string')
          return (
            <Field key={name} label={label} {...(hint ? { hint } : {})}>
              {choices.length ? (
                <WorkflowChoiceSelect
                  disabled={disabled}
                  value={typeof value[name] === 'string' ? value[name] : ''}
                  placeholder="Choose a value"
                  aria-required={isRequired}
                  options={choices.map((entry) => ({
                    value: entry,
                    label: optionLabel(name, entry),
                  }))}
                  onChange={(next) => change(next)}
                />
              ) : (
                <input
                  required={required.includes(name)}
                  disabled={disabled}
                  value={typeof value[name] === 'string' ? value[name] : ''}
                  onChange={(event) => change(event.target.value)}
                />
              )}
            </Field>
          );
        return (
          <p key={name} role="alert">
            Unsupported parameter field: {label}. Update the webapp before configuring this action.
          </p>
        );
      })}
    </>
  );
}
