import { Suspense } from "react";
import JobsExplorer from "./JobsExplorer";
import { TableSkeleton } from "@/components/ui/Skeleton";

export default function JobsPage() {
  return (
    <Suspense fallback={<TableSkeleton rows={8} />}>
      <JobsExplorer />
    </Suspense>
  );
}
