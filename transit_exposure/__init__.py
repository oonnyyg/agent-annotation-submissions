"""Limb-darkened transit light curves with finite-exposure averaging."""

from .core import (
    LimbDarkening,
    TransitGeometry,
    instantaneous_flux,
    exposure_averaged_flux,
    external_contact_interval,
    total_stellar_flux,
)

__all__ = [
    "LimbDarkening",
    "TransitGeometry",
    "instantaneous_flux",
    "exposure_averaged_flux",
    "external_contact_interval",
    "total_stellar_flux",
]
