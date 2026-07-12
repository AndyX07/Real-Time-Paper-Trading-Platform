import { useEffect, useRef } from "react";
import { createChart, type IChartApi, type ISeriesApi, type UTCTimestamp } from "lightweight-charts";
import { useCandleSocket } from "../../hooks/useCandleSocket";
import type { CandleMessage } from "../../types/candle";

interface CandleChartProps {
  symbol: string;
  intervalMinutes: number;
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

export function CandleChart({ symbol, intervalMinutes }: CandleChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const backfillDoneRef = useRef(false);
  const bufferRef = useRef<CandleMessage[]>([]);
  const lastTimeRef = useRef<number>(-Infinity);

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
  }

  useEffect(() => {
    if (!containerRef.current) return;

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 400,
      layout: { background: { color: "#131722" }, textColor: "#d1d4dc", attributionLogo: false},
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

    const handleResize = () => {
      if (containerRef.current) {
        chart.applyOptions({ width: containerRef.current.clientWidth });
      }
    };
    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
    };
  }, []);

  // Backfill on symbol/interval change -- see file header for the
  // ordering guarantee this provides against the live-update race.
  useEffect(() => {
    backfillDoneRef.current = false;
    bufferRef.current = [];
    lastTimeRef.current = -Infinity;
    seriesRef.current?.setData([]);

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

  return <div ref={containerRef} style={{ width: "100%" }} />;
}
