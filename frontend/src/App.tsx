import { Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { FindingPage } from "./pages/FindingPage";
import { HomePage } from "./pages/HomePage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { ReviewPage } from "./pages/ReviewPage";

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<HomePage />} />
        <Route path="reviews/:id" element={<ReviewPage />} />
        <Route path="reviews/:id/findings/:findingId" element={<FindingPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
