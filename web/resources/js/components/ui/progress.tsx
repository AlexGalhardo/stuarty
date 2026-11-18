"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Progress as ProgressPrimitive } from "@base-ui/react/progress"

function Progress({
  className,
  value,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root>) {
  const filled = value ?? 0
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      value={value}
      className={cn(
        "relative flex h-1 w-full items-center overflow-x-hidden rounded-full bg-muted",
        className
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className="size-full flex-1 bg-primary transition-all"
        style={{ transform: `translateX(-${100 - filled}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
