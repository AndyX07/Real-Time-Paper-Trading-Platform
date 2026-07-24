import { useEffect, useRef, useState } from "react";
import { createChart, type IChartApi, type ISeriesApi, type MouseEventParams, type UTCTimestamp } from "lightweight-charts";
import { useCandleSocket } from "../../hooks/useCandleSocket";
import type { CandleMessage } from "../../types/candle";

interface CandleChartProps {
  symbol: string;
  intervalMinutes: number;
  priceDecimals: number;
}

interface HistoryBar {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
}

function toBar(c: HistoryBar) {
  return {
    time: c.time as UTCTimestamp,
    open: c.open,
    high: c.high,
    low: c.low,
    close: c.close,
  };
}

export function CandleChart({ symbol, intervalMinutes, priceDecimals }: CandleChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const backfillDoneRef = useRef(false);
  const bufferRef = useRef<CandleMessage[]>([]);
  const lastTimeRef = useRef<number>(-Infinity);

  // Latest known bar (always current) vs. hovered bar (crosshair preview) --
  // the readout shows whichever is active, falling back to the latest once
  // the mouse leaves the chart, matching Kraken's own chart header.
  const [latestBar, setLatestBar] = useState<HistoryBar | null>(null);
  const [hoverBar, setHoverBar] = useState<HistoryBar | null>(null);

  // lightweight-charts throws if update() is called with a time earlier
  // than the series' current last bar -- guard every write through this
  // instead of calling series.update()/setData() directly, so a stray
  // out-of-order message (clock skew, a slow backfill racing live
  // messages, or Kraken's own catch-up burst of recent candles on a
  // fresh subscribe overlapping with the REST backfill) is dropped
  // instead of throwing and silently killing all subsequent updates.
  // This is expected to fire occasionally in normal operation -- debug
  // level, not a warning.
  function safeUpdate(bar: ReturnType<typeof toBar>) {
    if (bar.time < lastTimeRef.current) {
      console.debug("CandleChart: dropping out-of-order bar", bar, "last applied time was", lastTimeRef.current);
      return;
    }
    lastTimeRef.current = bar.time;
    seriesRef.current?.update(bar);
    setLatestBar(bar);
  }

  useEffect(() => {
    if (!containerRef.current) return;

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: containerRef.current.clientHeight,
      layout: { background: { color: "#131722" }, textColor: "#d1d4dc", attributionLogo: false },
      grid: {
        vertLines: { color: "#1e222d" },
        horzLines: { color: "#1e222d" },
      },
      timeScale: { timeVisible: true, secondsVisible: false },
    });
    const series = chart.addCandlestickSeries({
      upColor: "#26a69a",
      downColor: "#ef5350",
      borderVisible: false,
      wickUpColor: "#26a69a",
      wickDownColor: "#ef5350",
    });

    chartRef.current = chart;
    seriesRef.current = series;
    series.applyOptions({ priceFormat: { type: "price", precision: priceDecimals, minMove: 1 / 10 ** priceDecimals } });

    function handleCrosshairMove(param: MouseEventParams) {
      const data = param.time ? param.seriesData.get(series) : undefined;
      if (data && "open" in data) {
        const bar = data as unknown as { open: number; high: number; low: number; close: number };
        setHoverBar({ time: param.time as number, open: bar.open, high: bar.high, low: bar.low, close: bar.close });
      } else {
        setHoverBar(null);
      }
    }
    chart.subscribeCrosshairMove(handleCrosshairMove);

    const handleResize = () => {
      if (containerRef.current) {
        chart.applyOptions({ width: containerRef.current.clientWidth, height: containerRef.current.clientHeight });
      }
    };
    window.addEventListener("resize", handleResize);

    // The container's own height comes from the flex layout, not the
    // window resizing -- a plain window "resize" listener alone would miss
    // that, so also watch the container's box size directly.
    const resizeObserver = new ResizeObserver(handleResize);
    resizeObserver.observe(containerRef.current);

    return () => {
      window.removeEventListener("resize", handleResize);
      resizeObserver.disconnect();
      chart.unsubscribeCrosshairMove(handleCrosshairMove);
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
    };
  }, []);

  // The chart-creation effect above only runs once; re-apply price
  // precision whenever the selected symbol's own decimals change.
  useEffect(() => {
    seriesRef.current?.applyOptions({
      priceFormat: { type: "price", precision: priceDecimals, minMove: 1 / 10 ** priceDecimals },
    });
  }, [priceDecimals]);

  // Backfill on symbol/interval change -- see file header for the
  // ordering guarantee this provides against the live-update race.
  useEffect(() => {
    backfillDoneRef.current = false;
    bufferRef.current = [];
    lastTimeRef.current = -Infinity;
    seriesRef.current?.setData([]);
    setLatestBar(null);
    setHoverBar(null);

    let cancelled = false;

    async function loadHistory() {
      let history: HistoryBar[] = [];
      try {
        const res = await fetch(
          `http://localhost:8000/api/candles/history?symbol=${encodeURIComponent(symbol)}&interval=${intervalMinutes}&count=720`,
        );
        if (res.ok) {
          history = await res.json();
        }
      } catch {
        // backfill is best-effort -- the chart still works from live
        // updates alone if this fails
      }
      if (cancelled) return;

      const bars = history.map(toBar);
      seriesRef.current?.setData(bars);
      lastTimeRef.current = bars.length > 0 ? bars[bars.length - 1].time : -Infinity;
      setLatestBar(bars.length > 0 ? bars[bars.length - 1] : null);

      for (const c of bufferRef.current) {
        safeUpdate(toBar(c));
      }
      bufferRef.current = [];
      backfillDoneRef.current = true;
    }

    loadHistory();

    return () => {
      cancelled = true;
    };
  }, [symbol, intervalMinutes]);

  useCandleSocket(symbol, intervalMinutes, (candle: CandleMessage) => {
    if (!backfillDoneRef.current) {
      bufferRef.current.push(candle);
      return;
    }
    safeUpdate(toBar(candle));
  });

  const displayBar = hoverBar ?? latestBar;
  const change = displayBar ? displayBar.close - displayBar.open : null;
  const changePct = displayBar && displayBar.open ? (change! / displayBar.open) * 100 : null;
  const changeColor = change == null ? "text-text-muted" : change >= 0 ? "text-buy" : "text-sell";

  return (
    <div className="relative h-full w-full">
      {displayBar && (
        <div className="pointer-events-none absolute left-2 top-1 z-10 flex flex-wrap items-baseline gap-2 text-xs">
          <span className="font-semibold text-text-primary">{symbol}</span>
          <span className="text-text-muted">
            O<span className="text-text-primary">{displayBar.open.toFixed(priceDecimals)}</span> H
            <span className="text-text-primary">{displayBar.high.toFixed(priceDecimals)}</span> L
            <span className="text-text-primary">{displayBar.low.toFixed(priceDecimals)}</span> C
            <span className="text-text-primary">{displayBar.close.toFixed(priceDecimals)}</span>
          </span>
          {change != null && changePct != null && (
            <span className={changeColor}>
              {change >= 0 ? "+" : ""}
              {change.toFixed(priceDecimals)} ({changePct >= 0 ? "+" : ""}
              {changePct.toFixed(2)}%)
            </span>
          )}
        </div>
      )}
      <div ref={containerRef} className="h-full w-full" />
    </div>
  );
}
