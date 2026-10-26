import { useState } from 'react';
import { Dialog } from '../../shared/ui/dialog';
import { Button } from '../../shared/ui/button';

export type SessionMode = 'continue' | 'new';

export function useSessionChoice() {
  const [resolve, setResolve] = useState<((mode: SessionMode | null) => void) | null>(null);
  const finish = (mode: SessionMode | null) => {
    resolve?.(mode);
    setResolve(null);
  };
  return {
    choose: () => new Promise<SessionMode | null>((answer) => setResolve(() => answer)),
    dialog: (
      <Dialog
        title="Start work"
        description="Continue the saved conversation or start fresh. Your task files and previous chat will be kept."
        open={resolve !== null}
        onOpenChange={(open) => {
          if (!open) finish(null);
        }}
      >
        <div className="actions spacer">
          <Button onClick={() => finish('continue')}>Continue session</Button>
          <Button variant="outline" onClick={() => finish('new')}>
            New session
          </Button>
        </div>
      </Dialog>
    ),
  };
}
