import { useMemo, useRef, useState, type FormEvent } from 'react';
import Ajv from 'ajv';
import addFormats from 'ajv-formats';
import { Button } from '../../shared/ui/button';
import { ErrorState } from '../../shared/ui/common';
import {
  fieldOptions,
  type ChatQuestion,
  type FormField,
  type QuestionAnswer,
} from './question-model';

type Props = {
  question: ChatQuestion;
  canAnswer: boolean;
  respond: (answer: QuestionAnswer, submissionId: string) => Promise<void>;
};

export function QuestionCard({ question, canAnswer, respond }: Props) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>();
  const [recorded, setRecorded] = useState(false);
  const submission = useRef<{ payload: string; id: string } | null>(null);
  const editable = canAnswer && question.status === 'pending' && !recorded;
  const validate = useMemo(() => {
    const ajv = new Ajv({ allErrors: true, strict: false });
    addFormats(ajv);
    const schema = { ...question.schema } as Record<string, unknown>;
    delete schema.$schema;
    return ajv.compile(schema);
  }, [question.schema]);

  async function send(answer: QuestionAnswer) {
    const payload = JSON.stringify(answer);
    if (submission.current?.payload !== payload)
      submission.current = { payload, id: crypto.randomUUID() };
    setPending(true);
    setError(undefined);
    try {
      await respond(answer, submission.current.id);
      setRecorded(true);
    } catch (reason) {
      setError(reason);
    } finally {
      setPending(false);
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!validate(values)) {
      setError(
        new Error(
          validate.errors
            ?.map((error) => `${error.instancePath || 'Answer'} ${error.message}`)
            .join('; ') ?? 'Check your answer.',
        ),
      );
      return;
    }
    void send({ action: 'accept', content: values });
  }

  return (
    <section className="agent-approval agent-question" aria-label="Agent question">
      <strong>
        {question.status === 'pending' && !recorded ? 'Your input is needed' : 'Agent question'}
      </strong>
      <p>{question.message}</p>
      {editable ? (
        <form onSubmit={submit}>
          <fieldset disabled={pending}>
            {Object.entries(question.schema.properties).map(([name, field]) => (
              <label key={name} className="agent-question-field">
                <span>
                  {field.title ?? name}
                  {question.schema.required?.includes(name) ? ' *' : ''}
                </span>
                {field.description && <small>{field.description}</small>}
                <QuestionInput
                  field={field}
                  name={name}
                  required={question.schema.required?.includes(name) ?? false}
                  value={Object.hasOwn(values, name) ? values[name] : undefined}
                  onChange={(value) =>
                    setValues((previous) => {
                      const next = { ...previous, [name]: value };
                      if (value === undefined) delete next[name];
                      return next;
                    })
                  }
                />
              </label>
            ))}
            <div className="actions">
              <Button type="submit" size="sm">
                {pending ? 'Saving…' : 'Send answer'}
              </Button>
              <Button variant="outline" size="sm" onClick={() => void send({ action: 'decline' })}>
                Decline
              </Button>
              <Button variant="ghost" size="sm" onClick={() => void send({ action: 'cancel' })}>
                Cancel question
              </Button>
            </div>
          </fieldset>
        </form>
      ) : (
        <QuestionOutcome question={question} recorded={recorded} />
      )}
      {error != null && <ErrorState error={error} />}
    </section>
  );
}

function QuestionOutcome({ question, recorded }: { question: ChatQuestion; recorded: boolean }) {
  if (question.status === 'uncertain')
    return <p role="status">Question delivery is unresolved. Review recovery before continuing.</p>;
  if (question.answer)
    return (
      <div>
        <p>
          {question.answer.action === 'accept'
            ? 'Answer recorded'
            : question.answer.action === 'decline'
              ? 'Declined'
              : 'Cancelled'}
        </p>
        {Object.entries(question.answer.content ?? {}).map(([key, value]) => (
          <p key={key}>
            <strong>{question.schema.properties[key]?.title ?? key}: </strong>
            {Array.isArray(value) ? value.join(', ') : String(value)}
          </p>
        ))}
      </div>
    );
  return (
    <p role="status">
      {recorded
        ? 'Answer recorded'
        : question.status === 'cancelled'
          ? 'Question closed'
          : 'Waiting for an answer'}
    </p>
  );
}

function QuestionInput({
  field,
  name,
  required,
  value,
  onChange,
}: {
  field: FormField;
  name: string;
  required: boolean;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const options = fieldOptions(field.type === 'array' ? field.items! : field);
  if (field.type === 'boolean')
    return (
      <select
        name={name}
        required={required}
        value={typeof value === 'boolean' ? String(value) : ''}
        onChange={(event) =>
          onChange(event.target.value === '' ? undefined : event.target.value === 'true')
        }
      >
        <option value="">Select…</option>
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
    );
  if (field.type === 'array') {
    if (options.length)
      return (
        <select
          name={name}
          multiple
          required={required}
          value={(value as string[] | undefined) ?? []}
          onChange={(event) =>
            onChange(Array.from(event.target.selectedOptions, (option) => option.value))
          }
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      );
    return (
      <textarea
        name={name}
        required={required}
        placeholder="One answer per line"
        value={((value as string[] | undefined) ?? []).join('\n')}
        onChange={(event) =>
          onChange(event.target.value ? event.target.value.split('\n') : undefined)
        }
      />
    );
  }
  if (options.length)
    return (
      <SingleChoiceInput
        name={name}
        required={required}
        options={options}
        value={value as string | undefined}
        onChange={onChange}
      />
    );
  const numeric = field.type === 'integer' || field.type === 'number';
  return (
    <input
      name={name}
      required={required}
      type={numeric ? 'number' : 'text'}
      step={field.type === 'integer' ? 1 : 'any'}
      min={field.minimum}
      max={field.maximum}
      minLength={field.minLength}
      maxLength={field.maxLength}
      value={(value as string | number | undefined) ?? ''}
      onChange={(event) =>
        onChange(
          event.target.value === ''
            ? undefined
            : numeric
              ? Number(event.target.value)
              : event.target.value,
        )
      }
    />
  );
}

function SingleChoiceInput({
  name,
  required,
  options,
  value,
  onChange,
}: {
  name: string;
  required: boolean;
  options: { value: string; label: string }[];
  value: string | undefined;
  onChange: (value: string | undefined) => void;
}) {
  const selected = options.findIndex((option) => option.value === value);
  return (
    <select
      name={name}
      required={required}
      value={selected < 0 ? '' : String(selected)}
      onChange={(event) =>
        onChange(event.target.value === '' ? undefined : options[Number(event.target.value)]?.value)
      }
    >
      <option value="">Select…</option>
      {options.map((option, index) => (
        <option key={index} value={String(index)}>
          {option.label}
        </option>
      ))}
    </select>
  );
}
