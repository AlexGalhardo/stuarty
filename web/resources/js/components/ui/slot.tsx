"use client"

import * as React from "react"
import { useRender } from "@base-ui/react/use-render"

// Merges props onto the single child, the same job Radix Slot did for asChild.
function SlotRoot({
  children,
  ...props
}: React.HTMLAttributes<HTMLElement> & { children?: React.ReactNode }) {
  const child = React.isValidElement(children) ? children : undefined
  return useRender({
    defaultTagName: "div",
    render: child,
    props: {
      ...props,
      children: child ? undefined : children,
    },
  })
}

function childRender(
  asChild: boolean | undefined,
  children: React.ReactNode,
): React.ReactElement<Record<string, unknown>> | undefined {
  if (asChild && React.isValidElement(children)) {
    return children as React.ReactElement<Record<string, unknown>>
  }
  return undefined
}

const Slot = { Root: SlotRoot }

export { Slot, childRender }
