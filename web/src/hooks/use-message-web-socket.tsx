import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

import { GetRoomMessagesResponse } from '../http/get-room-messages'

interface UseMessageWebSocketParams {
  roomId: string
}

type WebhookMessage =
  | { kind: 'message_created'; value: { id: string; message: string } }
  | { kind: 'message_answered'; value: { id: string } }
  | { kind: 'message_reaction_increased'; value: { id: string; count: number } }
  | { kind: 'message_reaction_decreased'; value: { id: string; count: number } }

export const useMessageWebSocket = ({ roomId }: UseMessageWebSocketParams) => {
  const queryClient = useQueryClient()

  useEffect(() => {
    const ws = new WebSocket(`ws://localhost:8080/subscribe/${roomId}`)

    ws.onopen = () => {
      console.log('WebSocket connected')
    }

    ws.onmessage = (event) => {
      const data: WebhookMessage = JSON.parse(event.data)

      switch (data.kind) {
        case 'message_created': {
          queryClient.setQueryData<GetRoomMessagesResponse>(
            ['@ama/messages', roomId],
            (state) => {
              return {
                messages: [
                  ...(state?.messages ?? []),
                  {
                    id: data.value.id,
                    text: data.value.message,
                    amountOfReaction: 0,
                    answered: false,
                  },
                ],
              }
            },
          )

          break
        }
        case 'message_answered': {
          queryClient.setQueryData<GetRoomMessagesResponse>(
            ['@ama/messages', roomId],
            (state) => {
              if (!state) {
                return undefined
              }

              return {
                messages: state.messages.map((message) => {
                  if (message.id === data.value.id) {
                    return {
                      ...message,
                      answered: true,
                    }
                  }

                  return message
                }),
              }
            },
          )

          break
        }
        case 'message_reaction_increased':
        case 'message_reaction_decreased': {
          queryClient.setQueryData<GetRoomMessagesResponse>(
            ['@ama/messages', roomId],
            (state) => {
              if (!state) {
                return undefined
              }

              return {
                messages: state.messages.map((message) => {
                  if (message.id === data.value.id) {
                    return {
                      ...message,
                      amountOfReaction: data.value.count,
                    }
                  }

                  return message
                }),
              }
            },
          )
        }
      }
    }

    return () => {
      ws.close()
    }
  }, [roomId, queryClient])
}
