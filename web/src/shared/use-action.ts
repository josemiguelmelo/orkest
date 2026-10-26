import { useState } from 'react';
// Mutations stay outside the query cache: write-only credentials are transient.
export function useAction() {
  const [error, setError] = useState<unknown>();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState('');
  async function run(action: () => Promise<unknown>, success?: string) {
    setPending(true);
    setError(undefined);
    setMessage('');
    try {
      await action();
      setMessage(success ?? '');
      return true;
    } catch (reason) {
      setError(reason);
      return false;
    } finally {
      setPending(false);
    }
  }
  return { error, pending, message, run };
}
