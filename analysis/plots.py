"""Figures from the parking twin outputs.

    python analysis/plots.py outputs docs/figures
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
import pandas as pd  # noqa: E402

INK, MUTED, GRID = "#0b0b0b", "#52514e", "#e4e3df"
SERIES = ["#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#6250d6"]
CLUSTERS = ["academic", "admin", "hospital", "market", "sports"]


def style(ax):
    for s in ("top", "right"):
        ax.spines[s].set_visible(False)
    for s in ("left", "bottom"):
        ax.spines[s].set_color(GRID)
    ax.tick_params(colors=MUTED, labelsize=9)
    ax.grid(True, axis="y", color=GRID, linewidth=0.8)
    ax.set_axisbelow(True)


def scenario_comparison(out: Path, figs: Path):
    df = pd.read_csv(out / "scenario_comparison.csv")
    fig, axes = plt.subplots(1, 2, figsize=(11, 3.8), sharey=True)
    for ax, col, label in ((axes[0], "unmet_car_pct", "Car arrivals with no bay within 400 m (%)"),
                           (axes[1], "zones_over_85pct", "Destination zones at 85% or more at peak")):
        ax.barh(df["scenario"], df[col], color="#2a78d6", height=0.55)
        for y, v in enumerate(df[col]):
            ax.text(v, y, f"  {v:g}", va="center", fontsize=9, color=INK)
        style(ax)
        ax.grid(True, axis="x", color=GRID, linewidth=0.8)
        ax.grid(False, axis="y")
        ax.set_title(label, loc="left", fontsize=10.5, color=INK)
    axes[0].invert_yaxis()
    fig.tight_layout()
    fig.savefig(figs / "scenario_comparison.png", dpi=150)
    plt.close(fig)


def cluster_day(out: Path, figs: Path, scenario: str = "growth_2030"):
    occ = pd.read_csv(out / scenario / "occupancy.csv")
    occ = occ[occ["cluster"].isin(CLUSTERS)]
    g = occ.groupby(["time", "cluster"])[["cars", "car_bays"]].sum().reset_index()
    g["util"] = 100 * g["cars"] / g["car_bays"]
    pivot = g.pivot(index="time", columns="cluster", values="util")[CLUSTERS]
    fig, ax = plt.subplots(figsize=(10, 4.2))
    x = range(len(pivot))
    for color, cl in zip(SERIES, CLUSTERS):
        ax.plot(x, pivot[cl], color=color, linewidth=2, label=cl)
    ax.axhline(85, color=MUTED, linewidth=1, linestyle="--")
    ax.text(len(pivot) - 1, 86, "85% (practical capacity)", ha="right", fontsize=8, color=MUTED)
    ticks = list(range(0, len(pivot), 12))
    ax.set_xticks(ticks)
    ax.set_xticklabels(pivot.index[ticks])
    ax.set_ylim(0, 105)
    ax.set_ylabel("Car bays in use (%)", color=MUTED)
    style(ax)
    ax.set_title(f"Car park use by destination over the day ({scenario.replace('_', ' ')})", loc="left",
                 fontsize=11.5, color=INK)
    ax.legend(ncol=5, frameon=False, fontsize=8.5, loc="upper left", bbox_to_anchor=(0, -0.1))
    fig.tight_layout()
    fig.savefig(figs / "cluster_day.png", dpi=150)
    plt.close(fig)


def zone_map(out: Path, figs: Path, scenario: str = "growth_2030"):
    zones = pd.read_csv(out / "zones.csv")
    anchors = pd.read_csv(out / "anchors.csv")
    occ = pd.read_csv(out / scenario / "occupancy.csv")
    peak = occ.groupby("zone").apply(lambda d: ((d["cars"] + d["overflow_cars"]) / d["car_bays"].clip(lower=1)).max(),
                                     include_groups=False)
    zones["peak_util"] = zones["name"].map(peak).fillna(0) * 100
    dest = zones[zones["cluster"] != "residential"]
    home = zones[zones["cluster"] == "residential"]

    fig, ax = plt.subplots(figsize=(8.4, 8))
    ax.scatter(anchors["x"], anchors["y"], marker="s", s=28, color="#c3c2b7", label="accommodation anchor", zorder=1)
    sc = ax.scatter(dest["x"], dest["y"], s=dest["car_bays"] * 2.2, c=dest["peak_util"].clip(upper=150),
                    cmap="Blues", vmin=0, vmax=150, edgecolor="white", linewidth=1.5, zorder=2,
                    label="destination zone (size: car bays)")
    ax.scatter(home["x"], home["y"], s=home["car_bays"].clip(lower=4) * 2.2, c=home["peak_util"].clip(upper=150),
               cmap="Blues", vmin=0, vmax=150, marker="D", edgecolor="white", linewidth=1, zorder=2,
               label="residential zone")
    for cl, grp in dest.groupby("cluster"):
        over = int((grp["peak_util"] >= 100).sum())
        note = f"{over} of {len(grp)} zones full" if over else "spare bays at peak"
        ax.annotate(f"{cl.capitalize()}\n{note}", (grp["x"].max(), grp["y"].mean()), xytext=(14, 0),
                    textcoords="offset points", fontsize=8, color=INK, va="center")
    cb = fig.colorbar(sc, ax=ax, fraction=0.035, pad=0.01)
    cb.set_label("Peak demand as % of car bays (overflow included)", color=MUTED, fontsize=9)
    cb.outline.set_visible(False)
    ax.set_aspect("equal")
    ax.set_axis_off()
    ax.set_title(f"Peak car parking demand by zone ({scenario.replace('_', ' ')})", loc="left", fontsize=11.5,
                 color=INK)
    ax.legend(frameon=False, fontsize=8, loc="lower left")
    fig.tight_layout()
    fig.savefig(figs / "zone_map.png", dpi=150)
    plt.close(fig)


def main(out="outputs", figs="docs/figures"):
    out, figs = Path(out), Path(figs)
    figs.mkdir(parents=True, exist_ok=True)
    scenario_comparison(out, figs)
    cluster_day(out, figs)
    zone_map(out, figs)
    summary = json.loads((out / "baseline" / "summary.json").read_text())
    print(f"figures written to {figs} (baseline peak {summary['peak_time']})")


if __name__ == "__main__":
    main(*sys.argv[1:])
