package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Sup3r-Us3r/ama-app/internal/store/pgstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type apiHandler struct {
	q           *pgstore.Queries
	r           *chi.Mux
	upgrader    websocket.Upgrader
	subscribers map[string]map[*websocket.Conn]context.CancelFunc
	mu          *sync.Mutex
}

func (h apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.r.ServeHTTP(w, r)
}

func NewHandler(q *pgstore.Queries) http.Handler {
	apiHandler := apiHandler{
		q:           q,
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		subscribers: make(map[string]map[*websocket.Conn]context.CancelFunc),
		mu:          &sync.Mutex{},
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/subscribe/{room_id}", apiHandler.handleSubscribe)

	r.Route("/api", func(r chi.Router) {
		r.Route("/rooms", func(r chi.Router) {
			r.Post("/", apiHandler.handleCreateRoom)
			r.Get("/", apiHandler.handleGetRooms)

			r.Route("/{room_id}", func(r chi.Router) {
				r.Get("/", apiHandler.handleGetRoom)

				r.Route("/messages", func(r chi.Router) {
					r.Post("/", apiHandler.handleCreateRoomMessage)
					r.Get("/", apiHandler.handleGetRoomMessages)

					r.Route("/{message_id}", func(r chi.Router) {
						r.Get("/", apiHandler.handleGetRoomMessage)
						r.Patch("/react", apiHandler.handleReactToMessage)
						r.Delete("/react", apiHandler.handleRemoveReactFromMessage)
						r.Patch("/answer", apiHandler.handleMarkMessageAsAnswered)
					})
				})
			})
		})
	})

	apiHandler.r = r

	return apiHandler
}

const (
	MessageKindMessageCreated           = "message_created"
	MessageKindMessageReactionIncreased = "message_reaction_increased"
	MessageKindMessageReactionDecreased = "message_reaction_decreased"
	MessageKindMessageAnswered          = "message_answered"
)

type MessageMessageReactionIncreased struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

type MessageMessageReactionDecreased struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
}

type MessageMessageAnswered struct {
	ID string `json:"id"`
}

type MessageMessageCreated struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type Message struct {
	Kind   string `json:"kind"`
	Value  any    `json:"value"`
	RoomID string `json:"-"`
}

func (h apiHandler) notifyClients(msg Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	subscribers, ok := h.subscribers[msg.RoomID]
	if !ok || len(subscribers) == 0 {
		return
	}

	for connection, cancel := range subscribers {
		if err := connection.WriteJSON(msg); err != nil {
			slog.Error("failed to send message to client", "error", err)
			cancel()
		}
	}
}

func (h apiHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	_, rawRoomID, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	connection, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("failed to upgrade connection", "error", err)
		http.Error(w, "failed to upgrade to ws connection", http.StatusBadRequest)
		return
	}

	defer connection.Close()

	ctx, cancel := context.WithCancel(r.Context())

	h.mu.Lock()
	if _, ok := h.subscribers[rawRoomID]; !ok {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
	}
	slog.Info("new client connected", "room_id", rawRoomID, "client_ip", r.RemoteAddr)
	h.subscribers[rawRoomID][connection] = cancel
	h.mu.Unlock()

	<-ctx.Done()

	h.mu.Lock()
	delete(h.subscribers[rawRoomID], connection)
	h.mu.Unlock()
}

func (h apiHandler) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	type _body struct {
		Theme string `json:"theme"`
	}

	var body _body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	roomID, err := h.q.InsertRoom(r.Context(), body.Theme)
	if err != nil {
		slog.Error("failed to insert room", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	type response struct {
		ID string `json:"id"`
	}

	sendJSON(w, response{ID: roomID.String()})
}

func (h apiHandler) handleGetRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.q.GetRooms(r.Context())
	if err != nil {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		slog.Error("failed to get rooms", "error", err)
		return
	}

	if rooms == nil {
		rooms = []pgstore.Room{}
	}

	sendJSON(w, rooms)
}

func (h apiHandler) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	room, _, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	sendJSON(w, room)
}

func (h apiHandler) handleCreateRoomMessage(w http.ResponseWriter, r *http.Request) {
	_, rawRoomID, roomID, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	type _body struct {
		Message string `json:"message"`
	}

	var body _body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	messageID, err := h.q.InsertMessage(r.Context(), pgstore.InsertMessageParams{RoomID: roomID, Message: body.Message})
	if err != nil {
		slog.Error("failed to insert message", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	type response struct {
		ID string `json:"id"`
	}

	sendJSON(w, response{ID: messageID.String()})

	go h.notifyClients(Message{
		Kind:   MessageKindMessageCreated,
		RoomID: rawRoomID,
		Value: MessageMessageCreated{
			ID:      messageID.String(),
			Message: body.Message,
		},
	})
}

func (h apiHandler) handleGetRoomMessages(w http.ResponseWriter, r *http.Request) {
	_, _, roomID, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	messages, err := h.q.GetRoomMessages(r.Context(), roomID)
	if err != nil {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		slog.Error("failed to get room messages", "error", err)
		return
	}

	if messages == nil {
		messages = []pgstore.Message{}
	}

	sendJSON(w, messages)
}

func (h apiHandler) handleGetRoomMessage(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	rawMessageID := chi.URLParam(r, "message_id")
	messageID, err := uuid.Parse(rawMessageID)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}

	messages, err := h.q.GetMessage(r.Context(), messageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "message not found", http.StatusBadRequest)
			return
		}

		slog.Error("failed to get message", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	sendJSON(w, messages)
}

func (h apiHandler) handleReactToMessage(w http.ResponseWriter, r *http.Request) {
	_, rawRoomID, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	rawID := chi.URLParam(r, "message_id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}

	count, err := h.q.ReactToMessage(r.Context(), id)
	if err != nil {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		slog.Error("failed to react to message", "error", err)
		return
	}

	type response struct {
		Count int64 `json:"count"`
	}

	sendJSON(w, response{Count: count})

	go h.notifyClients(Message{
		Kind:   MessageKindMessageReactionIncreased,
		RoomID: rawRoomID,
		Value: MessageMessageReactionIncreased{
			ID:    rawID,
			Count: count,
		},
	})
}

func (h apiHandler) handleRemoveReactFromMessage(w http.ResponseWriter, r *http.Request) {
	_, rawRoomID, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	rawID := chi.URLParam(r, "message_id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}

	count, err := h.q.RemoveReactionFromMessage(r.Context(), id)
	if err != nil {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		slog.Error("failed to react to message", "error", err)
		return
	}

	type response struct {
		Count int64 `json:"count"`
	}

	sendJSON(w, response{Count: count})

	go h.notifyClients(Message{
		Kind:   MessageKindMessageReactionDecreased,
		RoomID: rawRoomID,
		Value: MessageMessageReactionDecreased{
			ID:    rawID,
			Count: count,
		},
	})
}

func (h apiHandler) handleMarkMessageAsAnswered(w http.ResponseWriter, r *http.Request) {
	_, rawRoomID, _, ok := h.readRoom(w, r)
	if !ok {
		return
	}

	rawID := chi.URLParam(r, "message_id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}

	err = h.q.MarkMessageAsAnswered(r.Context(), id)
	if err != nil {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		slog.Error("failed to react to message", "error", err)
		return
	}

	w.WriteHeader(http.StatusOK)

	go h.notifyClients(Message{
		Kind:   MessageKindMessageAnswered,
		RoomID: rawRoomID,
		Value: MessageMessageAnswered{
			ID: rawID,
		},
	})
}
