import React, { useEffect, useState } from 'react';
import {
  Modal,
  ModalOverlay,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Text,
  Box,
  IconButton,
  CloseButton,
} from '@chakra-ui/react';
import { CopyIcon } from '@chakra-ui/icons';
import { v4 as uuidv4 } from 'uuid';

const LocalRunModal: React.FC = () => {
  const { isOpen, onOpen, onClose } = useDisclosure();
  const [uniqueId, setUniqueId] = useState<string>('');

  useEffect(() => {
    if (isOpen) {
      setUniqueId(uuidv4());
    }
  }, [isOpen]);

  const codeSnippets = [
    'brew install kurtosis',
    `kurtosis run local ${uniqueId}`
  ];

  const handleCopy = (snippet: string): void => {
    navigator.clipboard.writeText(snippet)
      .then(() => console.log('Code copied to clipboard!'))
      .catch(err => console.error('Failed to copy: ', err));
  };

  return (
    <>
      <Button size={"xs"} variant={"ghost"}
        onClick={onOpen}>
        Run locally
      </Button>

      <Modal isOpen={isOpen} onClose={onClose} size="xl" isCentered>
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>Setup local environment</ModalHeader>
          <CloseButton position="absolute" right="8px" top="8px" onClick={onClose} />
          <ModalBody>
            <Text>Follow the steps below deploy a local copy:</Text>
            {codeSnippets.map((snippet, index) => (
              <Box key={index} as="pre" p="4" background="gray.500" my="2" position="relative" overflowY="auto">
                {snippet}
                <IconButton
                  aria-label="Copy code"
                  icon={<CopyIcon />}
                  size="sm"
                  position="absolute"
                  right="1"
                  top="1"
                  onClick={() => handleCopy(snippet)}
                />
              </Box>
            ))}
          </ModalBody>
        </ModalContent>
      </Modal>
    </>
  );
};

export default LocalRunModal;

