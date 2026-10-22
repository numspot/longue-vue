import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ExtractButton } from './ExtractButton';

const emptyExtract = () => Promise.resolve({ truncated: false, filename: null });

describe('ExtractButton', () => {
  it('renders disabled when prop says so', () => {
    render(<ExtractButton label="Extract" disabled onExtract={emptyExtract} />);
    expect(screen.getByRole('button', { name: /extract/i })).toBeDisabled();
  });

  it('opens a menu with CSV and JSON when clicked', async () => {
    const user = userEvent.setup();
    render(<ExtractButton label="Extract" onExtract={emptyExtract} />);
    await user.click(screen.getByRole('button', { name: /extract/i }));
    expect(screen.getByRole('menuitem', { name: /csv/i })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: /json/i })).toBeInTheDocument();
  });

  it('invokes onExtract with the chosen format and reports truncation', async () => {
    const user = userEvent.setup();
    const onExtract = vi.fn().mockResolvedValue({ truncated: true, filename: 'assets.csv' });
    const onTruncation = vi.fn();
    render(<ExtractButton label="Extract" onExtract={onExtract} onTruncation={onTruncation} />);
    await user.click(screen.getByRole('button', { name: /extract/i }));
    await user.click(screen.getByRole('menuitem', { name: /csv/i }));
    expect(onExtract).toHaveBeenCalledWith('csv');
    await waitFor(() => expect(onTruncation).toHaveBeenCalledWith(true));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('supports keyboard opening and Escape with focus restored', async () => {
    const user = userEvent.setup();
    render(<ExtractButton label="Extract" onExtract={emptyExtract} />);
    const trigger = screen.getByRole('button', { name: /extract/i });
    trigger.focus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menu')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });
});
