// Free hosting puts the server to sleep when idle; this tracks requests that are waiting for it to wake.
let waiting = 0;
const listeners = new Set<() => void>();

function emit() {
  for (const listener of listeners) listener();
}

/** Call when a request starts waiting for the server to wake; call the returned function when it stops. */
export function beginWaking(): () => void {
  waiting++;
  emit();
  let done = false;
  return () => {
    if (done) return;
    done = true;
    waiting--;
    emit();
  };
}

export function subscribeWaking(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function isWaking(): boolean {
  return waiting > 0;
}
