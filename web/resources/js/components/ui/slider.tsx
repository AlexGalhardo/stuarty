"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Slider as SliderPrimitive } from "@base-ui/react/slider"

function thumbValues(
  value: number | readonly number[] | undefined,
  defaultValue: number | readonly number[] | undefined,
  min: number,
  max: number,
): readonly number[] {
  if (Array.isArray(value)) return value
  if (typeof value === "number") return [value]
  if (Array.isArray(defaultValue)) return defaultValue
  if (typeof defaultValue === "number") return [defaultValue]
  return [min, max]
}

function Slider({
  className,
  defaultValue,
  value,
  min = 0,
  max = 100,
  step,
  disabled,
  orientation,
  onValueChange,
  onValueCommit,
  "aria-label": ariaLabel,
}: {
  className?: string
  defaultValue?: number[]
  value?: number[]
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  orientation?: "horizontal" | "vertical"
  onValueChange?: (value: number[]) => void
  onValueCommit?: (value: number[]) => void
  "aria-label"?: string
}) {
  const values = React.useMemo(
    () => thumbValues(value, defaultValue, min, max),
    [value, defaultValue, min, max],
  )
  const asList = (next: number | readonly number[]): number[] =>
    typeof next === "number" ? [next] : [...next]

  return (
    <SliderPrimitive.Root
      data-slot="slider"
      defaultValue={defaultValue}
      value={value}
      min={min}
      max={max}
      step={step}
      disabled={disabled}
      orientation={orientation}
      aria-label={ariaLabel}
      onValueChange={(next) => onValueChange?.(asList(next))}
      onValueCommitted={(next) => onValueCommit?.(asList(next))}
      className={cn(
        "relative flex w-full touch-none items-center select-none data-disabled:opacity-50 data-vertical:h-full data-vertical:min-h-40 data-vertical:w-auto data-vertical:flex-col",
        className
      )}
    >
      <SliderPrimitive.Control className="relative flex w-full grow items-center data-vertical:h-full data-vertical:flex-col">
        <SliderPrimitive.Track
          data-slot="slider-track"
          className="relative grow overflow-hidden rounded-full bg-muted data-horizontal:h-1 data-horizontal:w-full data-vertical:h-full data-vertical:w-1"
        >
          <SliderPrimitive.Indicator
            data-slot="slider-range"
            className="absolute bg-primary select-none data-horizontal:h-full data-vertical:w-full"
          />
        </SliderPrimitive.Track>
        {values.map((_, index) => (
          <SliderPrimitive.Thumb
            data-slot="slider-thumb"
            index={index}
            key={index}
            getAriaLabel={ariaLabel ? () => ariaLabel : undefined}
            className="relative block size-6 shrink-0 rounded-full ring-ring/50 before:absolute before:inset-1.5 before:rounded-full before:border before:border-ring before:bg-white transition-[color,box-shadow] select-none hover:ring-3 focus-visible:ring-3 focus-visible:outline-hidden active:ring-3 disabled:pointer-events-none disabled:opacity-50"
          />
        ))}
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  )
}

export { Slider }
