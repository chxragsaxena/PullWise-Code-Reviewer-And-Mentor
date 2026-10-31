import { Link, Outlet } from "react-router-dom";
import { AuthStatus } from "./AuthStatus";
import { ThemeToggle } from "./ThemeToggle";
import { WakingBanner } from "./WakingBanner";
import "./Layout.css";

function BrandMark() {
  return (
    <svg className="brand__mark" viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="8" fill="currentColor" />
      <path
        d="M10 9v14M10 9a3 3 0 100 6 3 3 0 000-6zm12 8v6m0-6a3 3 0 100 6"
        fill="none"
        stroke="var(--accent-contrast)"
        strokeWidth="2.2"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function Layout() {
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <header className="app-header">
        <div className="container app-header__inner">
          <Link to="/" className="brand">
            <BrandMark />
            <span>PullWise</span>
          </Link>
          <span className="app-header__tagline">Code review that teaches</span>
          <Link to="/ask" className="app-header__link">
            Ask
          </Link>
          <AuthStatus />
          <ThemeToggle />
        </div>
      </header>
      <WakingBanner />
      <main id="main" className="container app-main">
        <Outlet />
      </main>
      <footer className="app-footer">
        <div className="container">Every finding comes with evidence and a lesson.</div>
      </footer>
    </div>
  );
}
