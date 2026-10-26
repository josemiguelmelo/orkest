import { it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Field } from './form';
it('labels selects independently of option text and connects help/errors', () => {
  render(
    <Field label="Repository" hint="Choose a registered repository." error="Required">
      <select defaultValue="">
        <option value="">Choose repository</option>
        <option value="repo">Hello-World</option>
      </select>
    </Field>,
  );
  const select = screen.getByRole('combobox', { name: 'Repository' });
  expect(screen.getByLabelText('Repository', { exact: true })).toBe(select);
  expect(select).toHaveAccessibleDescription('Choose a registered repository. Required');
  expect(select).toHaveAttribute('aria-invalid', 'true');
});
