import { useState, useId, type FormEvent } from 'react';
import {
  AlertCircle,
  Check,
  CheckCircle2,
  Circle,
  Eye,
  EyeOff,
  KeyRound,
  UserRound,
} from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import { ApiError, command, data } from '../../shared/api/client';
import { Button } from '../../shared/ui/button';

export function UserSettingsPage() {
  const { session } = useApp();
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [validationError, setValidationError] = useState<'length' | 'same' | 'mismatch' | ''>('');
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<{ kind: 'success' | 'error'; message: string } | null>(null);
  const passwordLongEnough = newPassword.length >= 12;
  const passwordIsDifferent = !!newPassword && !!currentPassword && newPassword !== currentPassword;
  const confirmationMatches = !!confirmPassword && newPassword === confirmPassword;

  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setNotice(null);
    setValidationError('');
    if (newPassword.length < 12) {
      setValidationError('length');
      return;
    }
    if (newPassword === currentPassword) {
      setValidationError('same');
      return;
    }
    if (newPassword !== confirmPassword) {
      setValidationError('mismatch');
      return;
    }
    setSaving(true);
    try {
      await data(
        sdk.changeLocalPassword({
          ...command,
          body: { currentPassword, newPassword },
        }),
      );
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setNotice({ kind: 'success', message: 'Your password was changed successfully.' });
    } catch (cause) {
      setNotice({
        kind: 'error',
        message: passwordErrorMessage(cause),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <header className="page-header">
        <div>
          <p className="eyebrow">ACCOUNT SETTINGS</p>
          <h1>Your account</h1>
          <p className="muted">Review your profile and manage how you sign in.</p>
        </div>
      </header>
      <div className="user-settings-grid">
        <section className="user-account-summary" aria-label="Your account profile">
          <span className="user-account-avatar" aria-hidden="true">
            {session.user.username.slice(0, 2).toUpperCase()}
          </span>
          <div className="user-account-copy">
            <strong>{session.user.username}</strong>
            <span>{session.user.email}</span>
          </div>
          <span className="user-account-method">
            <UserRound size={14} />
            {session.hasLocalPassword ? 'Username and password' : 'Keycloak sign-in'}
          </span>
        </section>

        <section className="user-settings-card" aria-labelledby="password-heading">
          <div className="user-settings-card-heading">
            <span className="user-settings-icon">
              <KeyRound size={18} />
            </span>
            <div>
              <h2 id="password-heading">Sign-in password</h2>
              <p className="muted">Choose a strong password you don’t use elsewhere.</p>
            </div>
          </div>
          {session.hasLocalPassword ? (
            <form className="form-stack user-password-form" onSubmit={changePassword}>
              {notice?.kind === 'success' && (
                <div className="user-settings-success" role="status" aria-live="polite">
                  <CheckCircle2 size={18} />
                  <span>{notice.message}</span>
                </div>
              )}
              {notice?.kind === 'error' && (
                <div className="user-settings-error" role="alert">
                  <AlertCircle size={18} />
                  <div>
                    <strong>Password not changed</strong>
                    <p>{notice.message}</p>
                  </div>
                </div>
              )}
              <PasswordField
                label="Current password"
                autoComplete="current-password"
                value={currentPassword}
                disabled={saving}
                onChange={(value) => {
                  setCurrentPassword(value);
                  setNotice(null);
                  setValidationError('');
                }}
              />
              <PasswordField
                label="New password"
                autoComplete="new-password"
                value={newPassword}
                disabled={saving}
                onChange={(value) => {
                  setNewPassword(value);
                  setNotice(null);
                  setValidationError('');
                }}
                error={
                  validationError === 'length'
                    ? 'Enter at least 12 characters.'
                    : validationError === 'same'
                      ? 'Choose a password different from your current one.'
                      : undefined
                }
              />
              <PasswordField
                label="Confirm new password"
                autoComplete="new-password"
                value={confirmPassword}
                disabled={saving}
                onChange={(value) => {
                  setConfirmPassword(value);
                  setNotice(null);
                  setValidationError('');
                }}
                error={validationError === 'mismatch' ? 'The passwords do not match.' : undefined}
              />
              <ul className="password-requirements" aria-label="Password requirements">
                <PasswordRequirement met={passwordLongEnough}>
                  At least 12 characters
                </PasswordRequirement>
                <PasswordRequirement met={passwordIsDifferent}>
                  Different from your current password
                </PasswordRequirement>
                <PasswordRequirement met={confirmationMatches}>
                  New password entries match
                </PasswordRequirement>
              </ul>
              <div className="form-actions">
                <Button type="submit" disabled={saving}>
                  {saving ? 'Saving password…' : 'Save new password'}
                </Button>
              </div>
            </form>
          ) : (
            <p className="user-settings-note">
              This account signs in with Keycloak. Manage its password with your identity provider.
            </p>
          )}
        </section>
      </div>
    </>
  );
}

function PasswordField({
  label,
  autoComplete,
  value,
  onChange,
  disabled,
  error,
}: {
  label: string;
  autoComplete: 'current-password' | 'new-password';
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  error?: string | undefined;
}) {
  const id = useId();
  const [visible, setVisible] = useState(false);
  const describedBy = error ? `${id}-error` : undefined;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <div className="password-input-wrap">
        <input
          id={id}
          type={visible ? 'text' : 'password'}
          autoComplete={autoComplete}
          maxLength={1024}
          value={value}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
          aria-invalid={!!error}
          aria-describedby={describedBy}
          required
        />
        <button
          type="button"
          className="password-visibility"
          aria-label={`${visible ? 'Hide' : 'Show'} ${label.toLowerCase()}`}
          aria-pressed={visible}
          disabled={disabled}
          onClick={() => setVisible((shown) => !shown)}
        >
          {visible ? <EyeOff size={16} /> : <Eye size={16} />}
        </button>
      </div>
      {error && (
        <span id={`${id}-error`} className="field-error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}

function PasswordRequirement({ met, children }: { met: boolean; children: string }) {
  return (
    <li data-met={met}>
      {met ? <Check size={14} aria-hidden="true" /> : <Circle size={12} aria-hidden="true" />}
      <span>{children}</span>
    </li>
  );
}

function passwordErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return 'We couldn’t reach Orkest to confirm the change. Check your connection and try again.';
  }
  switch (error.code) {
    case 'current_password_invalid':
      return 'That current password doesn’t match your account. Check it and try again.';
    case 'conflict':
      return 'Your password was updated elsewhere. Enter the latest current password and try again.';
    case 'invalid_request':
      return 'Check that your new password has at least 12 characters and matches the confirmation.';
    case 'local_password_unavailable':
      return 'This account uses Keycloak sign-in. Manage its password with your identity provider.';
    default:
      return error.status >= 500
        ? 'Orkest couldn’t save your password right now. Please try again in a moment.'
        : 'We couldn’t change your password. Check the fields and try again.';
  }
}
