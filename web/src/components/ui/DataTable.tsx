import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type Column,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";

type DataTableProps<TData> = {
  ariaLabel: string;
  columns: ColumnDef<TData, unknown>[];
  data: TData[];
  emptyState: ReactNode;
  getRowId: (row: TData, index: number) => string;
  minTableWidth?: number;
  defaultSorting?: SortingState;
  onRowClick?: (row: TData) => void;
  rowClassName?: (row: TData) => string | undefined;
};

export function DataTable<TData>({
  ariaLabel,
  columns,
  data,
  emptyState,
  getRowId,
  minTableWidth,
  defaultSorting = [],
  onRowClick,
  rowClassName,
}: DataTableProps<TData>) {
  const [sorting, setSorting] = useState<SortingState>(defaultSorting);
  const [scrollState, setScrollState] = useState({ canScroll: false, atStart: true, atEnd: true });
  const scrollRef = useRef<HTMLDivElement>(null);
  const pinnedColumns = useMemo(() => {
    const ids = columns.map((column, index) => {
      const definition = column as { id?: string; accessorKey?: string | number };
      return definition.id ?? String(definition.accessorKey ?? `column-${index}`);
    });
    return { left: ids.slice(0, 1), right: ids.length > 1 ? ids.slice(-1) : [] };
  }, [columns]);
  const table = useReactTable({
    columns,
    data,
    getCoreRowModel: getCoreRowModel(),
    getRowId,
    getSortedRowModel: getSortedRowModel(),
    onSortingChange: setSorting,
    state: { columnPinning: pinnedColumns, sorting },
  });

  useEffect(() => {
    const container = scrollRef.current;
    if (!container) return;
    const updateScrollState = () => {
      const maximum = container.scrollWidth - container.clientWidth;
      setScrollState({
        canScroll: maximum > 1,
        atStart: container.scrollLeft <= 1,
        atEnd: container.scrollLeft >= maximum - 1,
      });
    };
    updateScrollState();
    container.addEventListener("scroll", updateScrollState, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(updateScrollState);
    observer?.observe(container);
    if (container.firstElementChild) observer?.observe(container.firstElementChild);
    return () => { container.removeEventListener("scroll", updateScrollState); observer?.disconnect(); };
  }, [columns.length, data.length]);

  return <div className="data-table" ref={scrollRef}>
    <table aria-label={ariaLabel} className="data-table-table" style={minTableWidth ? { minWidth: minTableWidth } : undefined}>
      <thead>{table.getHeaderGroups().map((headerGroup) => <tr key={headerGroup.id}>{headerGroup.headers.map((header) => {
        const pinned = header.column.getIsPinned();
        return <th key={header.id} className={pinningClassName(pinned, scrollState, true)} style={columnStyle(header.column)}>{header.isPlaceholder ? null : header.column.getCanSort()
          ? <button className="data-table-sort-button" type="button" onClick={header.column.getToggleSortingHandler()}>{flexRender(header.column.columnDef.header, header.getContext())}<SortIndicator column={header.column} /></button>
          : flexRender(header.column.columnDef.header, header.getContext())}</th>;
      })}</tr>)}</thead>
      <tbody>{table.getRowModel().rows.length === 0
        ? <tr><td className="data-table-empty" colSpan={columns.length}>{emptyState}</td></tr>
        : table.getRowModel().rows.map((row) => <tr key={row.id} className={rowClassName?.(row.original)} onClick={onRowClick ? () => onRowClick(row.original) : undefined}>{row.getVisibleCells().map((cell) => {
          const pinned = cell.column.getIsPinned();
          return <td key={cell.id} className={pinningClassName(pinned, scrollState)} style={columnStyle(cell.column)}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>;
        })}</tr>)}</tbody>
    </table>
  </div>;
}

function SortIndicator<TData>({ column }: { column: Column<TData, unknown> }) {
  const direction = column.getIsSorted();
  return direction === "asc" ? <ArrowUp size={14} aria-hidden="true" /> : direction === "desc" ? <ArrowDown size={14} aria-hidden="true" /> : <ArrowUpDown size={14} aria-hidden="true" />;
}

function pinningClassName(pinned: false | "left" | "right", scrollState: { canScroll: boolean; atStart: boolean; atEnd: boolean }, header = false) {
  const classes = ["data-table-cell"];
  if (header) classes.push("data-table-cell--header");
  if (pinned) classes.push(`data-table-cell--pinned-${pinned}`);
  if (pinned === "left" && scrollState.canScroll && !scrollState.atStart) classes.push("data-table-cell--pinned-left-shadow");
  if (pinned === "right" && scrollState.canScroll && !scrollState.atEnd) classes.push("data-table-cell--pinned-right-shadow");
  return classes.join(" ");
}

function pinningStyle<TData>(column: Column<TData, unknown>) {
  const pinned = column.getIsPinned();
  if (pinned === "left") return { left: column.getStart("left"), position: "sticky" as const };
  if (pinned === "right") return { right: column.getAfter("right"), position: "sticky" as const };
  return undefined;
}

function columnStyle<TData>(column: Column<TData, unknown>) {
  const pinning = pinningStyle(column);
  const size = column.columnDef.size;
  if (size === undefined) return pinning;
  return { ...pinning, width: size, minWidth: column.columnDef.minSize ?? size, maxWidth: column.columnDef.maxSize ?? size };
}
