import { Select } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import type { ReactNode, Ref } from "react";

export type SelectFieldOption = {
  value: string;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
};

export function SelectField({
  value,
  onChange,
  options,
  ariaLabel,
  id,
  name,
  disabled = false,
  className,
  triggerRef,
}: {
  value: string;
  onChange: (value: string) => void;
  options: SelectFieldOption[];
  ariaLabel: string;
  id?: string;
  name?: string;
  disabled?: boolean;
  className?: string;
  triggerRef?: Ref<HTMLButtonElement>;
}) {
  const selected = options.find((option) => option.value === value);

  return (
    <Select.Root<string>
      value={value}
      onValueChange={(nextValue) => onChange(nextValue ?? "")}
      disabled={disabled}
      name={name}
    >
      <Select.Trigger
        ref={triggerRef}
        id={id}
        className={`select-field__trigger${className ? ` ${className}` : ""}`}
        aria-label={ariaLabel}
      >
        <Select.Value className="select-field__value">
          {selected?.label ?? "请选择"}
        </Select.Value>
        <Select.Icon className="select-field__icon"><ChevronDown size={16} aria-hidden="true" /></Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Positioner sideOffset={6} className="select-field__positioner">
          <Select.Popup className="select-field__popup">
            <Select.List className="select-field__list">
              {options.map((option) => (
                <Select.Item
                  key={option.value}
                  value={option.value}
                  label={typeof option.label === "string" ? option.label : option.value}
                  disabled={option.disabled}
                  className="select-field__option"
                >
                  <span className="select-field__option-copy">
                    <Select.ItemText className="select-field__option-label">{option.label}</Select.ItemText>
                    {option.description ? <span className="select-field__option-description">{option.description}</span> : null}
                  </span>
                  <Select.ItemIndicator className="select-field__option-check"><Check size={15} aria-hidden="true" /></Select.ItemIndicator>
                </Select.Item>
              ))}
            </Select.List>
          </Select.Popup>
        </Select.Positioner>
      </Select.Portal>
    </Select.Root>
  );
}
