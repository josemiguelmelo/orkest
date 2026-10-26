import { cloneElement, isValidElement, useId, type ReactNode } from 'react';
import type { FieldValues, UseFormReturn } from 'react-hook-form';
import { ErrorState } from './common';
import { Button } from './button';

export function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string;
  error?: string | undefined;
  hint?: string;
  children: ReactNode;
}) {
  const id = useId();
  const describedBy = [hint ? `${id}-hint` : '', error ? `${id}-error` : '']
    .filter(Boolean)
    .join(' ');
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {isValidElement<{ id?: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean }>(
        children,
      )
        ? cloneElement(children, {
            id,
            ...(describedBy ? { 'aria-describedby': describedBy } : {}),
            'aria-invalid': !!error,
          })
        : children}
      {hint && (
        <small id={`${id}-hint`} className="muted">
          {hint}
        </small>
      )}
      {error && (
        <span id={`${id}-error`} className="field-error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}

export function Form<T extends FieldValues>({
  form,
  onSubmit,
  children,
  error,
  label = 'Save changes',
}: {
  form: UseFormReturn<T>;
  onSubmit: (values: T) => Promise<void>;
  children: ReactNode;
  error: unknown;
  label?: string;
}) {
  return (
    <form className="form" onSubmit={form.handleSubmit(onSubmit)}>
      {children}
      {error !== undefined && error !== null && <ErrorState error={error} />}
      <div className="form-actions">
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? 'Saving…' : label}
        </Button>
      </div>
    </form>
  );
}
export function Selection({
  label,
  options,
  value,
  onChange,
  disabled = false,
}: {
  label: string;
  options: { id: string; name: string }[];
  value: string[];
  onChange: (value: string[]) => void;
  disabled?: boolean;
}) {
  return (
    <fieldset>
      <legend className="field">{label}</legend>
      <div className="check-list">
        {options.length ? (
          options.map((option) => (
            <label key={option.id}>
              <input
                type="checkbox"
                checked={value.includes(option.id)}
                disabled={disabled}
                onChange={(e) =>
                  onChange(
                    e.target.checked
                      ? [...value, option.id]
                      : value.filter((id) => id !== option.id),
                  )
                }
              />
              {option.name}
            </label>
          ))
        ) : (
          <span className="muted">None available yet.</span>
        )}
      </div>
    </fieldset>
  );
}
