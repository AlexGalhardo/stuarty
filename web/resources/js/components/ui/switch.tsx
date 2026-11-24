"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Switch as SwitchPrimitive } from "@base-ui/react/switch"

function Switch({
  className,
  size = "default",
  ...props
}: React.ComponentProps<typeof SwitchPrimitive.Root> & {
  size?: "sm" | "default"
}) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      data-size={size}
      className={cn(
        "peer group/switch relative inline-flex shrink-0 items-center rounded-full border border-transparent transition-all outline-none after:absolute after:-inset-x-3 after:-inset-y-2 max-md:after:-inset-y-3 aria-invalid:border-danger-text data-[size=default]:h-[18.4px] data-[size=default]:w-[32px] data-[size=sm]:h-[14px] data-[size=sm]:w-[24px] data-checked:bg-accent data-unchecked:border-border-strong data-unchecked:bg-surface-sunken data-disabled:cursor-not-allowed data-disabled:opacity-50",
        className
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none block translate-x-0 rounded-full bg-text-on-accent ring-0 transition-transform group-data-[size=default]/switch:size-4 group-data-[size=sm]/switch:size-3 in-data-checked:translate-x-[calc(100%-2px)] in-data-unchecked:bg-text-muted"
      />
    </SwitchPrimitive.Root>
  )
}

export { Switch }
