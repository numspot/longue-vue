import { render, screen, fireEvent } from '@testing-library/react';
import { SearchInput } from './SearchInput';

describe('SearchInput', () => {
  it('connects its label and clears the controlled search', () => {
    const onChange = vi.fn();
    render(<SearchInput value="prod" onChange={onChange} label="Cluster name" />);
    const input = screen.getByRole('searchbox', { name: 'Cluster name' });
    expect(input).toHaveAttribute('data-slot', 'search-input');
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }));
    expect(onChange).toHaveBeenCalledWith('');
    expect(input).toHaveFocus();
  });

  it('renders the glob hint and forwards changes', () => {
    const onChange = vi.fn();
    render(<SearchInput value="" onChange={onChange} />);
    const input = screen.getByPlaceholderText('Filter by name… (* = wildcard)');
    expect(input).toHaveAttribute('title', expect.stringContaining('du* = starts with'));
    fireEvent.change(input, { target: { value: 'prod-*' } });
    expect(onChange).toHaveBeenCalledWith('prod-*');
  });
});
