interface SkeletonProps {
  className?: string;
  style?: React.CSSProperties;
}

export function Skeleton({ className = "", style }: SkeletonProps) {
  return <div className={`skeleton ${className}`} style={style} aria-hidden />;
}

export function StatPanelSkeleton() {
  return (
    <div className="panel p-4">
      <Skeleton className="h-3 w-20 mb-3" />
      <Skeleton className="h-8 w-24 mb-2" />
      <Skeleton className="h-4 w-16" />
    </div>
  );
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="panel overflow-hidden">
      <Skeleton className="h-10 w-full rounded-none" />
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-none mt-px" />
      ))}
    </div>
  );
}
