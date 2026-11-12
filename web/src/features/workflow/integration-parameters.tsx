import { Field } from '../../shared/ui/form';
import type { JsonValue } from '../integrations/schema';

type Schema = Record<string, unknown>;
function object(value: unknown): Schema {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Schema)
    : {};
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
        const hint = typeof field.description === 'string' ? field.description : undefined;
        const change = (next: JsonValue) => onChange({ ...value, [name]: next });
        const choices = Array.isArray(field.enum)
          ? field.enum.filter((entry): entry is string => typeof entry === 'string')
          : [];
        if (name === 'artifactInputName')
          return (
            <Field key={name} label={label} {...(hint ? { hint } : {})}>
              <select
                disabled={disabled}
                required
                value={typeof value[name] === 'string' ? value[name] : ''}
                onChange={(event) => change(event.target.value)}
              >
                <option value="">Choose a stage artifact input</option>
                {typeof value[name] === 'string' &&
                  value[name] !== '' &&
                  !artifactInputs.includes(value[name]) && (
                    <option value={value[name]}>Unavailable artifact input ({value[name]})</option>
                  )}
                {artifactInputs.map((input) => (
                  <option key={input}>{input}</option>
                ))}
              </select>
            </Field>
          );
        if (field.type === 'boolean')
          return (
            <Field key={name} label={label} {...(hint ? { hint } : {})}>
              <input
                type="checkbox"
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
                <select
                  multiple
                  disabled={disabled}
                  value={selected}
                  onChange={(event) =>
                    change(Array.from(event.target.selectedOptions, (option) => option.value))
                  }
                >
                  {options
                    .filter((entry): entry is string => typeof entry === 'string')
                    .map((entry) => (
                      <option key={entry}>{entry}</option>
                    ))}
                </select>
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
                <select
                  required={required.includes(name)}
                  disabled={disabled}
                  value={typeof value[name] === 'string' ? value[name] : ''}
                  onChange={(event) => change(event.target.value)}
                >
                  <option value="">Choose a value</option>
                  {choices.map((entry) => (
                    <option key={entry}>{entry}</option>
                  ))}
                </select>
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
