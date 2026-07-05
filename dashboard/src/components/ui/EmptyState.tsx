import { LucideIcon } from "lucide-react";

interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description: string;
  action?: React.ReactNode;
}

export function EmptyState({ icon: Icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="panel p-12 text-center">
      {Icon && (
        <Icon
          size={32}
          strokeWidth={1.25}
          className="mx-auto mb-4"
          style={{ color: "var(--text-tertiary)" }}
        />
      )}
      <p className="h2 mb-2" style={{ color: "var(--text-secondary)" }}>
        {title}
      </p>
      <p className="body-sm mb-6" style={{ color: "var(--text-tertiary)", maxWidth: 360, margin: "0 auto 24px" }}>
        {description}
      </p>
      {action}
    </div>
  );
}
