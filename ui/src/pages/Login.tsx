import { FormEvent, useEffect, useState } from 'react';
import { Button, Callout, CalloutDescription, Input } from '@cloud-console/numspot-ds';
import { useNavigate, useSearchParams } from 'react-router';
import { ApiError, getAuthConfig, login } from '../api';
import { LogoIcon } from '../Logo';

// Session cookies stay server-owned. OIDC still uses the backend's authorize
// endpoint; the design-system migration changes presentation, not auth.
const OIDC_ERROR_MESSAGES: Record<string, string> = {
  state_expired_or_unknown: 'Sign-in link expired or already used. Try again.',
  exchange_failed: 'The identity provider rejected the sign-in. Try again or contact an admin.',
  user_lookup_failed: 'Could not load your user account. An admin should check longue-vue logs.',
  session_create_failed: 'Could not create your session. An admin should check longue-vue logs.',
  session_mint_failed: 'Could not mint a session id. An admin should check longue-vue logs.',
};

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [oidc, setOidc] = useState<{ enabled: boolean; label?: string } | null>(null);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  useEffect(() => {
    let cancelled = false;
    getAuthConfig()
      .then((cfg) => { if (!cancelled) setOidc(cfg.oidc); })
      .catch(() => { if (!cancelled) setOidc({ enabled: false }); });
    return () => { cancelled = true; };
  }, []);

  const oidcError = searchParams.get('oidc_error');
  const oidcErrorMessage = oidcError
    ? OIDC_ERROR_MESSAGES[oidcError] || `OIDC sign-in failed: ${oidcError}`
    : null;

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!username.trim() || !password) {
      setError('Username and password required.');
      return;
    }
    setBusy(true);
    try {
      await login(username.trim(), password);
      navigate('/clusters', { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message || 'Invalid credentials.');
      } else {
        setError(`Network error: ${String(err)}`);
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth-page">
      <form className="auth-card" onSubmit={onSubmit} noValidate>
        <div className="auth-header">
          <div className="auth-logo-ring"><LogoIcon size={48} /></div>
          <div className="auth-brand">
            <span className="auth-brand-light">longue</span>
            <span className="auth-brand-bold">-vue</span>
          </div>
          <div className="auth-tagline">CMDB · SecNumCloud</div>
        </div>
        <div className="auth-body">
          <h2 className="auth-title">Sign in</h2>
          <p className="auth-hint">
            First install? Check the <code>LONGUE-VUE FIRST-RUN BOOTSTRAP</code> banner
            in the startup log for the initial admin password.
          </p>
          {oidcErrorMessage && (
            <Callout intent="error" className="auth-feedback">
              <CalloutDescription>{oidcErrorMessage}</CalloutDescription>
            </Callout>
          )}
          <div className="auth-fields">
            <Input
              id="username"
              label="Username"
              type="text"
              required
              autoComplete="username"
              autoFocus
              placeholder="your username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
            <Input
              id="password"
              label="Password"
              type="password"
              required
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <Button className="auth-action" size="lg" type="submit" loading={busy} aria-label={busy ? 'Signing in…' : undefined}>
            {busy ? 'Signing in…' : 'Sign in'}
          </Button>
          {error && (
            <Callout intent="error" className="auth-feedback">
              <CalloutDescription>{error}</CalloutDescription>
            </Callout>
          )}
          {oidc?.enabled && (
            <>
              <div className="auth-divider"><span>or</span></div>
              <Button
                type="button"
                variant="outlined"
                className="auth-action"
                onClick={() => { window.location.href = '/v1/auth/oidc/authorize'; }}
              >
                Sign in with {oidc.label || 'OIDC'}
              </Button>
            </>
          )}
        </div>
      </form>
    </div>
  );
}
