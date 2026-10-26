import { Dialog as Primitive } from 'radix-ui';
import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from './button';

export function Dialog({
  title,
  description,
  children,
  open,
  onOpenChange,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Primitive.Root open={open} onOpenChange={onOpenChange}>
      <Primitive.Portal>
        <Primitive.Overlay className="dialog-overlay" />
        <Primitive.Content
          className="dialog-content"
          aria-describedby={description ? 'dialog-description' : undefined}
        >
          <div className="dialog-heading">
            <div>
              <Primitive.Title>{title}</Primitive.Title>
              {description && (
                <Primitive.Description id="dialog-description">{description}</Primitive.Description>
              )}
            </div>
            <Primitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label="Close">
                <X size={18} />
              </Button>
            </Primitive.Close>
          </div>
          {children}
        </Primitive.Content>
      </Primitive.Portal>
    </Primitive.Root>
  );
}
