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
        "peer relative flex size-6 shrink-0 items-center justify-center outline-none before:pointer-events-none before:absolute before:inset-1 before:rounded-[4px] before:border before:border-input before:transition-colors group-has-disabled/field:opacity-50 focus-visible:before:border-ring focus-visible:before:ring-3 focus-visible:before:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:before:border-destructive aria-invalid:before:ring-3 aria-invalid:before:ring-destructive/20 aria-invalid:aria-checked:before:border-primary dark:before:bg-input/30 dark:aria-invalid:before:border-destructive/50 dark:aria-invalid:before:ring-destructive/40 data-checked:before:border-primary data-checked:before:bg-primary data-checked:text-primary-foreground dark:data-checked:before:bg-primary",
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
