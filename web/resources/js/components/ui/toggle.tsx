"use client"

import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/cn"
import { Toggle as TogglePrimitive } from "@base-ui/react/toggle"

const toggleVariants = cva(
  "group/toggle inline-flex items-center justify-center gap-1 rounded-md border border-transparent text-sm font-medium whitespace-nowrap text-text-muted transition-colors outline-none hover:bg-surface-hover hover:text-text disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-danger-text aria-pressed:border-focus aria-pressed:bg-surface-selected aria-pressed:text-accent-text data-pressed:border-focus data-pressed:bg-surface-selected data-pressed:text-accent-text data-[state=on]:border-focus data-[state=on]:bg-surface-selected data-[state=on]:text-accent-text [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        outline: "border-border-strong bg-transparent",
      },
      size: {
        default:
          "h-8 min-w-8 px-2.5 max-md:h-11 max-md:min-w-11 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
        sm: "h-7 min-w-7 px-2.5 text-xs max-md:h-11 max-md:min-w-11 md:dashboard:h-8 md:dashboard:min-w-8 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 min-w-10 px-2.5 max-md:h-11 max-md:min-w-11 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Toggle({
  className,
  variant = "default",
  size = "default",
  ...props
}: React.ComponentProps<typeof TogglePrimitive> &
  VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive
      data-slot="toggle"
      className={cn(toggleVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Toggle, toggleVariants }
