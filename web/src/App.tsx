import { Navigate, Route, Routes } from "react-router-dom";
import Shell from "./layout/Shell";
import ApplicationDetail from "./pages/ApplicationDetail";
import Applications from "./pages/Applications";
import ImagePolicies from "./pages/ImagePolicies";
import Login from "./pages/Login";
import Repositories from "./pages/Repositories";
import Revisions from "./pages/Revisions";

/** The console routes. */
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<Shell />}>
        <Route path="/apps" element={<Applications />} />
        <Route path="/apps/:ns/:name/:tab?" element={<ApplicationDetail />} />
        <Route path="/repositories" element={<Repositories />} />
        <Route path="/revisions" element={<Revisions />} />
        <Route path="/imagepolicies" element={<ImagePolicies />} />
      </Route>
      <Route path="*" element={<Navigate to="/apps" replace />} />
    </Routes>
  );
}
