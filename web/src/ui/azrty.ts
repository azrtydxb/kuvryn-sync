// Typed re-exports of the vendored Azrty components. The .jsx files carry no
// types, and TypeScript infers every destructured prop as required, so the
// console imports them from here with their documented props instead.
import type {
  CSSProperties,
  FC,
  ReactNode,
  ButtonHTMLAttributes,
  InputHTMLAttributes,
} from "react";
import { Button as ButtonJSX } from "../azrty/components/actions/Button.jsx";
import { IconButton as IconButtonJSX } from "../azrty/components/actions/IconButton.jsx";
import { Icon as IconJSX } from "../azrty/components/brand/Icon.jsx";
import { Logo as LogoJSX } from "../azrty/components/brand/Logo.jsx";
import { ProductLogo as ProductLogoJSX } from "../azrty/components/brand/ProductLogo.jsx";
import { CodeBlock as CodeBlockJSX } from "../azrty/components/data/CodeBlock.jsx";
import { StatCard as StatCardJSX } from "../azrty/components/data/StatCard.jsx";
import { Alert as AlertJSX } from "../azrty/components/feedback/Alert.jsx";
import { Badge as BadgeJSX } from "../azrty/components/feedback/Badge.jsx";
import { EmptyState as EmptyStateJSX } from "../azrty/components/feedback/EmptyState.jsx";
import { Input as InputJSX } from "../azrty/components/forms/Input.jsx";
import { Select as SelectJSX } from "../azrty/components/forms/Select.jsx";
import { Tabs as TabsJSX } from "../azrty/components/navigation/Tabs.jsx";
import { Topbar as TopbarJSX } from "../azrty/components/navigation/Topbar.jsx";
import { PropertyList as PropertyListJSX } from "../azrty/components/panels/Drawer.jsx";

interface Styled {
  className?: string;
  style?: CSSProperties;
}

/** Tones the badge, alert and dot classes take. */
export type Tone =
  "good" | "warn" | "bad" | "info" | "neutral" | "outline" | "pillar";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "ghost" | "danger" | "accent";
  size?: "sm" | "md" | "lg";
  icon?: string;
  iconRight?: string;
  block?: boolean;
  children?: ReactNode;
}
export const Button = ButtonJSX as unknown as FC<ButtonProps>;

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: string;
  label: string;
  size?: number;
}
export const IconButton = IconButtonJSX as unknown as FC<IconButtonProps>;

export interface IconProps extends Styled {
  name: string;
  size?: number;
  color?: string;
  label?: string;
}
export const Icon = IconJSX as unknown as FC<IconProps>;

export interface LogoProps extends Styled {
  variant?: "lockup" | "mark";
  size?: number;
}
export const Logo = LogoJSX as unknown as FC<LogoProps>;

export interface ProductLogoProps extends Styled {
  name: string;
  sub?: string;
  tagline?: string;
  pillar?: string;
  emblem?: string;
  emblemLight?: string;
  layout?: "stacked" | "horizontal" | "icon";
  size?: number;
}
export const ProductLogo = ProductLogoJSX as unknown as FC<ProductLogoProps>;

export interface CodeBlockProps extends Styled {
  code: string;
  title?: string;
  copyable?: boolean;
  maxHeight?: number | string;
}
export const CodeBlock = CodeBlockJSX as unknown as FC<CodeBlockProps>;

export interface StatCardProps extends Styled {
  label: string;
  value: ReactNode;
  unit?: string;
  icon?: string;
  sub?: ReactNode;
}
export const StatCard = StatCardJSX as unknown as FC<StatCardProps>;

export interface AlertProps extends Styled {
  tone?: "info" | "good" | "warn" | "bad";
  title?: ReactNode;
  icon?: string;
  action?: ReactNode;
  children?: ReactNode;
}
export const Alert = AlertJSX as unknown as FC<AlertProps>;

export interface BadgeProps extends Styled {
  tone?: Tone;
  dot?: boolean;
  icon?: string;
  children?: ReactNode;
}
export const Badge = BadgeJSX as unknown as FC<BadgeProps>;

export interface EmptyStateProps extends Styled {
  icon?: string;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}
export const EmptyState = EmptyStateJSX as unknown as FC<EmptyStateProps>;

export interface InputProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "size"
> {
  label?: string;
  hint?: ReactNode;
  error?: ReactNode;
  icon?: string;
  mono?: boolean;
  size?: "sm" | "md";
}
export const Input = InputJSX as unknown as FC<InputProps>;

export interface SelectProps extends Omit<
  InputHTMLAttributes<HTMLSelectElement>,
  "size"
> {
  label?: string;
  hint?: ReactNode;
  error?: ReactNode;
  options: (string | { value: string; label: string })[];
  size?: "sm" | "md";
}
export const Select = SelectJSX as unknown as FC<SelectProps>;

export interface TabItem {
  id: string;
  label: string;
  count?: number;
}
export interface TabsProps extends Styled {
  items: TabItem[];
  value?: string;
  defaultValue?: string;
  onChange?: (id: string) => void;
}
export const Tabs = TabsJSX as unknown as FC<TabsProps>;

export interface TopbarProps extends Styled {
  crumbs: string[];
  live?: boolean;
  children?: ReactNode;
}
export const Topbar = TopbarJSX as unknown as FC<TopbarProps>;

export interface PropertyListProps extends Styled {
  items: { label: string; value: ReactNode; mono?: boolean }[];
}
export const PropertyList = PropertyListJSX as unknown as FC<PropertyListProps>;
