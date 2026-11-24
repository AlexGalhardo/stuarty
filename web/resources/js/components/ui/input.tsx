import * as React from "react"
import { cn } from "@/lib/cn"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-8 w-full min-w-0 rounded-md border border-border-strong bg-surface-sunken px-3 py-1 text-base text-text transition-colors outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-text placeholder:text-text-subtle focus-visible:border-focus disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-surface-hover disabled:opacity-60 aria-invalid:border-danger-text max-md:h-11 md:text-sm",
        className
      )}
      {...props}
    />
  )
}

export { Input }
