import { useSyncExternalStore } from "react";
import { isWaking, subscribeWaking } from "../api/waking";
import "./WakingBanner.css";

/** Shown while a request waits for the sleeping server, so a slow first load does not look broken. */
export function WakingBanner() {
  const waking = useSyncExternalStore(subscribeWaking, isWaking);
  if (!waking) return null;
  return (
    <div className="waking" role="status">
      The server was asleep and is waking up. This takes up to a minute, then everything runs normally.
    </div>
  );
}
