import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <main>
      <h1>Orkest</h1>
      <p>Agent orchestration platform. Project setup is ready; workflows are coming next.</p>
    </main>
  </StrictMode>,
);
