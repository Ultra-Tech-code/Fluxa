package fees

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fluxa/fluxa/internal/api"
	"github.com/fluxa/fluxa/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/", h.getSchedule)
		r.Post("/preview", h.previewFee)
	}
}

func (h *Handler) AdminRoutes() func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/collected", h.listCollected)
	}
}

type feeScheduleResponse struct {
	TransferFeeBps   int    `json:"transfer_fee_bps"`
	ConversionFeeBps int    `json:"conversion_fee_bps"`
	MinFeeAmount     string `json:"min_fee_amount"`
	MaxFeeAmount     string `json:"max_fee_amount,omitempty"`
	Asset            string `json:"asset"`
}

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.IDFromContext(r.Context())

	schedule, err := h.svc.GetSchedule(r.Context(), tenantID)
	if err != nil {
		api.HandleDomainError(w, err)
		return
	}

	resp := feeScheduleResponse{
		TransferFeeBps:   schedule.TransferFeeBps,
		ConversionFeeBps: schedule.ConversionFeeBps,
		MinFeeAmount:     schedule.MinFeeAmount.StringFixed(7),
		Asset:            schedule.Asset,
	}
	if schedule.MaxFeeAmount != nil {
		resp.MaxFeeAmount = schedule.MaxFeeAmount.StringFixed(7)
	}

	api.JSON(w, http.StatusOK, resp)
}

type previewReq struct {
	Type   string `json:"type" validate:"required,oneof=transfer conversion"`
	Asset  string `json:"asset" validate:"required"`
	Amount string `json:"amount" validate:"required"`
}

type previewResp struct {
	GrossAmount string `json:"gross_amount"`
	FeeAmount   string `json:"fee_amount"`
	NetAmount   string `json:"net_amount"`
	FeeBps      int    `json:"fee_bps"`
}

func (h *Handler) previewFee(w http.ResponseWriter, r *http.Request) {
	var req previewReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, "invalid request body")
		return
	}

	if err := api.Validate(req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		api.BadRequest(w, "amount must be a positive number")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	
	var fee *TransferFee
	if req.Type == "transfer" {
		fee, err = h.svc.CalculateTransferFee(r.Context(), tenantID, req.Asset, amount)
	} else {
		fee, err = h.svc.CalculateConversionFee(r.Context(), tenantID, req.Asset, amount)
	}

	if err != nil {
		api.HandleDomainError(w, err)
		return
	}

	api.JSON(w, http.StatusOK, previewResp{
		GrossAmount: amount.StringFixed(7),
		FeeAmount:   fee.FeeAmount.StringFixed(7),
		NetAmount:   fee.NetAmount.StringFixed(7),
		FeeBps:      fee.FeeBps,
	})
}


func (h *Handler) listCollected(w http.ResponseWriter, r *http.Request) {
	var start, end *time.Time
	if s := r.URL.Query().Get("start_date"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			api.BadRequest(w, "start_date must be RFC3339 format")
			return
		}
		start = &t
	}
	if e := r.URL.Query().Get("end_date"); e != "" {
		t, err := time.Parse(time.RFC3339, e)
		if err != nil {
			api.BadRequest(w, "end_date must be RFC3339 format")
			return
		}
		end = &t
	}

	summary, err := h.svc.ListCollectedSummary(r.Context(), start, end)
	if err != nil {
		api.InternalError(w, err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]interface{}{
		"summary": summary,
	})
}
