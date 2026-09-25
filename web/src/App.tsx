import { Navigate, Route, Routes } from "react-router-dom";
import Pending from "./pages/Pending";

/** The console routes. */
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Pending title="Welcome back" />} />
      <Route path="/apps" element={<Pending title="Applications" />} />
      <Route
        path="/apps/:ns/:name/:tab?"
        element={<Pending title="Application" />}
      />
      <Route path="/repositories" element={<Pending title="Repositories" />} />
      <Route path="/revisions" element={<Pending title="Revisions" />} />
      <Route
        path="/imagepolicies"
        element={<Pending title="Image policies" />}
      />
      <Route path="*" element={<Navigate to="/apps" replace />} />
    </Routes>
  );
}
