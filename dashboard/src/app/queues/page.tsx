import { Suspense } from "react";
import QueuesView from "./QueuesView";
import { TableSkeleton } from "@/components/ui/Skeleton";

export default function QueuesPage() {
  return (
    <Suspense fallback={<TableSkeleton rows={4} />}>
      <QueuesView />
    </Suspense>
  );
}
