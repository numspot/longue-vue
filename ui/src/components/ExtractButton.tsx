import { useState } from 'react';
import { Button, Menu, MenuContent, MenuItem, MenuTrigger } from '@cloud-console/numspot-ds';
import type { ExtractFormat } from '../api';

export interface ExtractButtonProps {
  label: string;
  disabled?: boolean;
  onExtract: (format: ExtractFormat) => Promise<{ truncated: boolean; filename: string | null }>;
  /** Called with the truncation flag from the server. Optional. */
  onTruncation?: (truncated: boolean) => void;
}

/** NumSpot's menu provides keyboard navigation, Escape, and outside dismissal. */
export function ExtractButton({ label, disabled, onExtract, onTruncation }: ExtractButtonProps) {
  const [busy, setBusy] = useState(false);

  const choose = async (format: ExtractFormat) => {
    setBusy(true);
    try {
      const result = await onExtract(format);
      onTruncation?.(result.truncated);
    } catch (e) {
      console.error('extract failed', e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Menu>
      <MenuTrigger asChild>
        <Button
          type="button"
          variant="outlined"
          size="sm"
          disabled={disabled}
          loading={busy}
          aria-label={busy ? `${label}…` : undefined}
        >
          {busy ? `${label}…` : label} ▾
        </Button>
      </MenuTrigger>
      <MenuContent align="end" onCloseAutoFocus={(event) => {
        // The DS's default prevents focus restoration; keyboard users need it.
        // When loading, the trigger is disabled until the download completes.
        if (busy) event.preventDefault();
      }}>
        <MenuItem onSelect={() => { void choose('csv'); }}>CSV</MenuItem>
        <MenuItem onSelect={() => { void choose('json'); }}>JSON</MenuItem>
      </MenuContent>
    </Menu>
  );
}
