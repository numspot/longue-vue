import { FormEvent, useState } from 'react';
import { Button, Callout, CalloutDescription, Input } from '@cloud-console/numspot-ds';
import { useNavigate } from 'react-router';
import { ApiError, changePassword } from '../api';

// Forced rotation per ADR-0007. A successful change invalidates all sessions,
// including this one, so the user signs in again with the new password.
export default function ChangePassword({ forced }: { forced?: boolean }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (next.length < 12) {
      setError('New password must be at least 12 characters.');
      return;
    }
    if (next !== confirm) {
      setError('Confirmation does not match.');
      return;
    }
    if (next === current) {
      setError('New password must differ from the current one.');
      return;
    }
    setBusy(true);
    try {
      await changePassword(current, next);
      navigate('/login', { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError(`Network error: ${String(err)}`);
      }
    } finally {
      setBusy(false);
    }
  };

  const strength = next.length === 0 ? 0 : next.length < 12 ? 1 : next.length < 16 ? 2 : 3;
  const strengthLabel = ['', 'Weak', 'Good', 'Strong'][strength];
  const strengthClass = ['', 'auth-strength-weak', 'auth-strength-good', 'auth-strength-strong'][strength];

  return (
    <div className="auth-page">
      <form className="auth-card" onSubmit={onSubmit} noValidate>
        <div className="auth-header">
          <div className="auth-logo-ring">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
              <path d="M7 11V7a5 5 0 0110 0v4" />
              <circle cx="12" cy="16" r="1.5" fill="var(--accent)" stroke="none" />
            </svg>
          </div>
          <div className="auth-brand">
            <span className="auth-brand-light">longue</span>
            <span className="auth-brand-bold">-vue</span>
          </div>
          <div className="auth-tagline">
            {forced ? 'Password rotation required' : 'Change your password'}
          </div>
        </div>
        <div className="auth-body">
          <h2 className="auth-title">
            {forced ? 'Rotate your password' : 'Change password'}
          </h2>
          {forced && (
            <p className="auth-hint">
              Your account is flagged <code>must_change_password</code>. Set a new password
              to unlock access — this is the only page available until you do.
            </p>
          )}
          <div className="auth-fields">
            <Input
              id="current"
              label="Current password"
              type="password"
              required
              autoComplete="current-password"
              autoFocus
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
            <div>
              <Input
                id="next"
                label="New password"
                description="12+ characters"
                aria-describedby="next-description"
                type="password"
                required
                autoComplete="new-password"
                value={next}
                onChange={(e) => setNext(e.target.value)}
              />
              {next.length > 0 && (
                <div className={`auth-strength ${strengthClass}`}>
                  <div className="auth-strength-bar">
                    <span style={{ width: `${(strength / 3) * 100}%` }} />
                  </div>
                  <span className="auth-strength-label">{strengthLabel}</span>
                </div>
              )}
            </div>
            <Input
              id="confirm"
              label="Confirm new password"
              type="password"
              required
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              success={confirm.length > 0 && next === confirm ? 'Passwords match' : undefined}
              aria-describedby={confirm.length > 0 && next === confirm ? 'confirm-success' : undefined}
            />
          </div>
          <Button className="auth-action" size="lg" type="submit" loading={busy} aria-label={busy ? 'Rotating…' : undefined}>
            {busy ? 'Rotating…' : forced ? 'Rotate password' : 'Change password'}
          </Button>
          {error && (
            <Callout intent="error" className="auth-feedback">
              <CalloutDescription>{error}</CalloutDescription>
            </Callout>
          )}
        </div>
      </form>
    </div>
  );
}
