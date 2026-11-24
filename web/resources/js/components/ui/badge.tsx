import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/cn"
import { childRender } from "@/components/ui/slot"
import { useRender } from "@base-ui/react/use-render"

const badgeVariants = cva(
  "group/badge inline-flex min-h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-colors has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 aria-invalid:border-danger-text [&>svg]:pointer-events-none [&>svg]:size-3!",
  {
    variants: {
      variant: {
        default:
          "bg-accent text-text-on-accent [a]:hover:bg-accent-hover",
        secondary:
          "border-border bg-surface-raised text-text [a]:hover:bg-surface-hover",
        destructive:
          "bg-danger-soft text-danger-text [a]:hover:bg-danger-soft",
        outline:
          "border-border-strong text-text [a]:hover:bg-surface-hover",
        ghost:
          "text-text-muted hover:bg-surface-hover hover:text-text",
        link: "text-accent-text underline-offset-4 hover:underline",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

function Badge({
  className,
  variant = "default",
  asChild = false,
  children,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const render = childRender(asChild, children)
  return useRender({
    defaultTagName: "span",
    render,
    props: {
      ...props,
      "data-slot": "badge",
      "data-variant": variant,
      className: cn(badgeVariants({ variant }), className),
      children: render ? undefined : children,
    },
  })
}

export { Badge, badgeVariants }
