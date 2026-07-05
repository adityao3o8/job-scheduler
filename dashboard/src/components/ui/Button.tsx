import { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "destructive" | "ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: "sm" | "md";
}

const VARIANT_CLASS: Record<Variant, string> = {
  primary: "btn-primary",
  secondary: "btn-secondary",
  destructive: "btn-destructive",
  ghost: "btn-ghost",
};

export function Button({
  variant = "primary",
  size = "md",
  className = "",
  children,
  ...props
}: ButtonProps) {
  const sizeClass = size === "sm" ? "text-[13px] px-3 py-1.5" : "";
  return (
    <button
      className={`btn ${VARIANT_CLASS[variant]} ${sizeClass} ${className}`}
      {...props}
    >
      {children}
    </button>
  );
}
