import { cn } from "@/lib/cn"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("rounded-md bg-surface-hover", className)}
      {...props}
    />
  )
}

export { Skeleton }
