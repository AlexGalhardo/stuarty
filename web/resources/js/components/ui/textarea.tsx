import * as React from "react"
import { cn } from "@/lib/cn"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-md border border-border-strong bg-surface-sunken px-3 py-2 text-base text-text transition-colors outline-none placeholder:text-text-subtle focus-visible:border-focus disabled:cursor-not-allowed disabled:bg-surface-hover disabled:opacity-60 aria-invalid:border-danger-text md:text-sm",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
