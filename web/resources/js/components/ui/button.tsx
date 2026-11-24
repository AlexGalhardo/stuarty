import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/cn"
import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { childRender } from "@/components/ui/slot"

const buttonVariants = cva(
  // The focus ring is the global 2 px outline of app.css (--color-focus), so
  // a control draws no halo of its own. Compact sizes grow to the 32 px target
  // on the dashboard and to 44 px at phone width (design-system section 7.8).
  "group/button inline-flex shrink-0 items-center justify-center rounded-md border border-transparent bg-clip-padding text-sm font-medium whitespace-nowrap transition-all outline-none select-none active:not-aria-[haspopup]:translate-y-px disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-danger-text [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        // The hover goes to the darker accent of the redesign instead of fading it.
        default:
          "bg-accent text-text-on-accent hover:bg-accent-hover",
        // The one primary action of a dashboard page (design-system section 11.6), never inside the editors.
        brand:
          "bg-brand text-brand-foreground hover:bg-[color-mix(in_oklab,var(--brand),black_14%)] dark:hover:bg-[color-mix(in_oklab,var(--brand),white_14%)]",
        outline:
          "border-border-strong bg-transparent text-text hover:bg-surface-hover aria-expanded:bg-surface-hover aria-pressed:border-focus aria-pressed:bg-surface-selected aria-pressed:text-accent-text",
        secondary:
          "border-border bg-surface-raised text-text hover:bg-surface-hover aria-expanded:bg-surface-hover aria-pressed:border-focus aria-pressed:bg-surface-selected aria-pressed:text-accent-text",
        ghost:
          "text-text-muted hover:bg-surface-hover hover:text-text aria-expanded:bg-surface-hover aria-expanded:text-text aria-pressed:border-focus aria-pressed:bg-surface-selected aria-pressed:text-accent-text",
        destructive:
          "bg-danger text-text-on-danger hover:bg-danger-hover",
        link: "text-accent-text underline-offset-4 hover:underline",
      },
      size: {
        default:
          "h-8 gap-1.5 px-3 max-md:h-11 has-data-[icon=inline-end]:pr-2.5 has-data-[icon=inline-start]:pl-2.5",
        xs: "h-6 gap-1 px-2 text-xs max-md:h-11 md:dashboard:h-8 in-data-[slot=button-group]:rounded-md has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-7 gap-1 px-2.5 text-xs max-md:h-11 md:dashboard:h-8 in-data-[slot=button-group]:rounded-md has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 gap-1.5 px-4 max-md:h-11 has-data-[icon=inline-end]:pr-3 has-data-[icon=inline-start]:pl-3",
        icon: "size-8 max-md:size-11",
        "icon-xs":
          "size-6 max-md:size-11 md:dashboard:size-8 in-data-[slot=button-group]:rounded-md [&_svg:not([class*='size-'])]:size-3",
        "icon-sm":
          "size-7 max-md:size-11 md:dashboard:size-8 in-data-[slot=button-group]:rounded-md",
        "icon-lg": "size-10 max-md:size-11",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  children,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const render = childRender(asChild, children)
  // Base UI gives a non-native button role="button"; a child with an href (an
  // anchor or an Inertia Link) is a link, and a link that announces itself as
  // a button breaks the name and the role that people and tests look for. Any
  // other child keeps the button role: a role={undefined} prop would erase it.
  const linkRole =
    render !== undefined && render.props.href !== undefined
      ? { role: "link" }
      : {}

  return (
    <ButtonPrimitive
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      nativeButton={render ? false : true}
      {...linkRole}
      render={render}
      {...props}
    >
      {render ? undefined : children}
    </ButtonPrimitive>
  )
}

export { Button, buttonVariants }
