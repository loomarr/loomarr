import type { PageHeaderProps } from "./page-header.type";

// PageHeader keeps the title hierarchy and explanatory copy consistent across route modules.
// Its caller still owns page layout: a full-height workspace can add its dividing border while
// a card page can place the same header inside a padded column.
const PageHeader = ({ title, description, className, ...rest }: PageHeaderProps) => (
  <header className={className} {...rest}>
    <h1 className="font-semibold text-xl">{title}</h1>
    {description ? <p className="mt-1 text-muted-foreground text-sm">{description}</p> : null}
  </header>
);

export { PageHeader };
