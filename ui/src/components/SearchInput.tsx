import { useId } from 'react';
import { Label, Search } from '@cloud-console/numspot-ds';

// Debouncing lives in useListControls. Keep the existing search contract while
// using NumSpot's search control (including its controlled clear action).
export function SearchInput({
  value,
  onChange,
  placeholder = 'Filter by name… (* = wildcard)',
  label = 'Name',
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  label?: string;
}) {
  const id = useId();
  return (
    <div className="list-search">
      <Label htmlFor={id}>{label}</Label>
      <Search
        id={id}
        size="md"
        value={value}
        placeholder={placeholder}
        title="Plain text matches anywhere in the name. Use * to anchor: du* = starts with, *du = ends with."
        onChange={(e) => onChange(e.target.value)}
        onClear={() => onChange('')}
      />
    </div>
  );
}
