import { Check, ChevronDown } from 'lucide-react';
import { DropdownMenu, Select } from 'radix-ui';

export type WorkflowChoice = {
  value: string;
  label: string;
  description?: string;
  disabled?: boolean;
};

export function WorkflowChoiceSelect({
  value,
  placeholder,
  options,
  groups,
  disabled,
  onChange,
  id,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  'aria-required': ariaRequired,
}: {
  value: string;
  placeholder: string;
  options?: WorkflowChoice[];
  groups?: { label: string; options: WorkflowChoice[] }[];
  disabled?: boolean;
  onChange: (value: string) => void;
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
}) {
  const renderOption = (option: WorkflowChoice) => (
    <Select.Item
      key={option.value}
      className="workflow-choice-item"
      value={option.value}
      disabled={option.disabled}
    >
      <span className="workflow-choice-copy">
        <Select.ItemText>{option.label}</Select.ItemText>
        {option.description && <small>{option.description}</small>}
      </span>
      <Select.ItemIndicator className="workflow-choice-check">
        <Check size={15} aria-hidden="true" />
      </Select.ItemIndicator>
    </Select.Item>
  );

  return (
    <Select.Root value={value} onValueChange={onChange} disabled={disabled}>
      <Select.Trigger
        id={id}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        aria-required={ariaRequired}
        className="workflow-choice-trigger"
      >
        <Select.Value placeholder={placeholder} />
        <Select.Icon className="workflow-choice-chevron">
          <ChevronDown size={16} aria-hidden="true" />
        </Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Content className="workflow-choice-content" position="popper" sideOffset={5}>
          <Select.Viewport>
            {groups?.map((group) => (
              <Select.Group key={group.label}>
                <Select.Label className="workflow-choice-group-label">{group.label}</Select.Label>
                {group.options.map(renderOption)}
              </Select.Group>
            ))}
            {options?.map(renderOption)}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  );
}

export function WorkflowMultiChoiceSelect({
  value,
  options,
  placeholder,
  disabled,
  onChange,
  id,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  'aria-required': ariaRequired,
}: {
  value: string[];
  options: WorkflowChoice[];
  placeholder: string;
  disabled?: boolean;
  onChange: (value: string[]) => void;
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
}) {
  const selectedLabels = options
    .filter((option) => value.includes(option.value))
    .map((option) => option.label);
  const summary =
    selectedLabels.length === 0
      ? placeholder
      : selectedLabels.length > 2
        ? `${selectedLabels.length} selected`
        : selectedLabels.join(', ');

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        id={id}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid}
        aria-required={ariaRequired}
        disabled={disabled}
        className="workflow-choice-trigger"
      >
        <span className={selectedLabels.length === 0 ? 'workflow-choice-placeholder' : ''}>
          {summary}
        </span>
        <ChevronDown className="workflow-choice-chevron" size={16} aria-hidden="true" />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          className="workflow-choice-content workflow-choice-multiple-content"
          align="start"
          sideOffset={5}
        >
          {options.map((option) => (
            <DropdownMenu.CheckboxItem
              key={option.value}
              className="workflow-choice-item"
              checked={value.includes(option.value)}
              disabled={option.disabled}
              onSelect={(event) => event.preventDefault()}
              onCheckedChange={(checked) =>
                onChange(
                  checked
                    ? [...value, option.value]
                    : value.filter((selected) => selected !== option.value),
                )
              }
            >
              <span className="workflow-choice-copy">{option.label}</span>
              <DropdownMenu.ItemIndicator className="workflow-choice-check">
                <Check size={15} aria-hidden="true" />
              </DropdownMenu.ItemIndicator>
            </DropdownMenu.CheckboxItem>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
