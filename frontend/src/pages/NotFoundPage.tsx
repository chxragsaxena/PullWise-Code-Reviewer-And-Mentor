import { Link } from "react-router-dom";
import { EmptyState } from "../components/PageState";

export function NotFoundPage() {
  return (
    <EmptyState title="Page not found">
      <Link to="/">Go back home</Link>
    </EmptyState>
  );
}
