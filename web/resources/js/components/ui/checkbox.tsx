"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox"
import { CheckIcon } from "lucide-react"

function Checkbox({
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "peer relative flex size-6 shrink-0 items-center justify-center outline-none before:pointer-events-none before:absolute before:inset-1 before:rounded-sm before:border before:border-border-strong before:bg-surface-sunken before:transition-colors after:absolute after:-inset-1 max-md:after:-inset-2.5 group-has-disabled/field:opacity-50 focus-visible:before:border-focus disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:before:border-danger-text aria-invalid:aria-checked:before:border-accent data-checked:before:border-accent data-checked:before:bg-accent data-checked:text-text-on-accent",
        className
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="relative grid place-content-center text-current transition-none [&>svg]:size-3.5"
      >
        <CheckIcon />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }
