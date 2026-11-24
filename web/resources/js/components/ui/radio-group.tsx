"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Radio } from "@base-ui/react/radio"
import { RadioGroup as RadioGroupPrimitive } from "@base-ui/react/radio-group"

function RadioGroup({
  className,
  onValueChange,
  ...props
}: React.ComponentProps<typeof RadioGroupPrimitive>) {
  return (
    <RadioGroupPrimitive
      data-slot="radio-group"
      className={cn("grid w-full gap-2", className)}
      onValueChange={(value, details) => onValueChange?.(value, details)}
      {...props}
    />
  )
}

function RadioGroupItem({
  className,
  ...props
}: React.ComponentProps<typeof Radio.Root>) {
  return (
    <Radio.Root
      data-slot="radio-group-item"
      className={cn(
        "group/radio-group-item peer relative flex aspect-square size-4 shrink-0 rounded-full border border-border-strong bg-surface-sunken outline-none after:absolute after:-inset-x-3 after:-inset-y-2 max-md:after:-inset-3 focus-visible:border-focus disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-danger-text aria-invalid:aria-checked:border-accent data-checked:border-accent data-checked:bg-accent data-checked:text-text-on-accent",
        className
      )}
      {...props}
    >
      <Radio.Indicator
        data-slot="radio-group-indicator"
        className="flex size-4 items-center justify-center"
      >
        <span className="absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 rounded-full bg-text-on-accent" />
      </Radio.Indicator>
    </Radio.Root>
  )
}

export { RadioGroup, RadioGroupItem }
